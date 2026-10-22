package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type BenchmarkReport struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	GeneratedAt   string `json:"generated_at_utc"`
	Results       struct {
		RecordsSubmitted                int     `json:"records_submitted"`
		RecordsReceived                 int     `json:"records_received"`
		RecordsFiltered                 int     `json:"records_filtered"`
		RecordsSentToDebugExporter      int     `json:"records_sent_to_debug_exporter"`
		RecordsArchived                 int     `json:"records_archived"`
		RecordReductionPercent          float64 `json:"record_reduction_percent"`
		OriginalOTLPJSONBytes           int64   `json:"original_otlp_json_bytes"`
		FilteredOTLPJSONEquivalentBytes int64   `json:"filtered_otlp_json_equivalent_bytes"`
		EstimatedBytesSaved             int64   `json:"estimated_bytes_saved"`
		EstimatedByteReductionPercent   float64 `json:"estimated_byte_reduction_percent"`
	} `json:"results"`
	Measurement struct {
		VendorWireBytesMeasured   bool   `json:"vendor_wire_bytes_measured"`
		MonetarySavingsCalculated bool   `json:"monetary_savings_calculated"`
		Note                      string `json:"note"`
	} `json:"measurement"`
}

type EstimateRequest struct {
	RunID          string  `json:"run_id"`
	PricePerGBUSD  float64 `json:"price_per_gb_usd"`
	BatchesPerDay  int64   `json:"benchmark_batches_per_day"`
	DaysPerMonth   int64   `json:"days_per_month"`
	PricingProfile string  `json:"pricing_profile,omitempty"`
}

type EstimateResponse struct {
	RunID          string `json:"run_id"`
	Currency       string `json:"currency"`
	PricingUnit    string `json:"pricing_unit"`
	PricingProfile string `json:"pricing_profile"`

	Assumptions struct {
		PricePerGBUSD float64 `json:"price_per_gb_usd"`
		BatchesPerDay int64   `json:"benchmark_batches_per_day"`
		DaysPerMonth  int64   `json:"days_per_month"`
		BytesPerGB    int64   `json:"bytes_per_gb"`
	} `json:"assumptions"`

	Monthly struct {
		OriginalEstimatedGB float64 `json:"original_estimated_gb"`
		FilteredEstimatedGB float64 `json:"filtered_estimated_gb"`
		EstimatedGBSaved    float64 `json:"estimated_gb_saved"`
		OriginalCostUSD     float64 `json:"original_cost_usd"`
		FilteredCostUSD     float64 `json:"filtered_cost_usd"`
		EstimatedSavingsUSD float64 `json:"estimated_savings_usd"`
		SavingsPercent      float64 `json:"savings_percent"`
	} `json:"monthly"`

	Disclaimer string `json:"disclaimer"`
}

type API struct {
	reportsDir string
}

var validRunID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func main() {
	reportsDir := os.Getenv("SIGNALFORGE_REPORTS_DIR")
	if reportsDir == "" {
		reportsDir = "reports/benchmarks"
	}

	addr := os.Getenv("SIGNALFORGE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	api := &API{reportsDir: reportsDir}
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", api.health)
	mux.HandleFunc("/api/v1/benchmarks", api.listBenchmarks)
	mux.HandleFunc("/api/v1/benchmarks/", api.getBenchmark)
	mux.HandleFunc("/api/v1/estimates", api.createEstimate)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5_000_000_000, // 5 seconds
	}

	log.Printf("[signalforge-api] listening on %s", addr)
	log.Printf("[signalforge-api] reports directory: %s", reportsDir)

	if err := server.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"service": "signalforge-control-plane",
	})
}

func (a *API) listBenchmarks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	files, err := filepath.Glob(filepath.Join(
		a.reportsDir,
		"signalforge-benchmark-*.json",
	))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Unable to list reports")
		return
	}

	reports := make([]BenchmarkReport, 0, len(files))

	for _, file := range files {
		report, err := readReport(file)
		if err != nil {
			log.Printf("[signalforge-api] invalid report %s: %v", file, err)
			writeError(w, http.StatusInternalServerError, "A benchmark report is invalid")
			return
		}
		reports = append(reports, report)
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].GeneratedAt > reports[j].GeneratedAt
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"count":      len(reports),
		"benchmarks": reports,
	})
}

func (a *API) getBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	runID := strings.TrimPrefix(r.URL.Path, "/api/v1/benchmarks/")

	if runID == "" || strings.Contains(runID, "/") ||
		!validRunID.MatchString(runID) {
		writeError(w, http.StatusBadRequest, "Invalid benchmark run ID")
		return
	}

	filename := "signalforge-benchmark-" + runID + ".json"
	report, err := readReport(filepath.Join(a.reportsDir, filename))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "Benchmark report not found")
			return
		}
		log.Printf("[signalforge-api] cannot read report %s: %v", runID, err)
		writeError(w, http.StatusInternalServerError, "Unable to read benchmark report")
		return
	}

	if report.RunID != runID {
		writeError(w, http.StatusNotFound, "Benchmark report not found")
		return
	}

	writeJSON(w, http.StatusOK, report)
}

func (a *API) createEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var req EstimateRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}

	if req.RunID == "" || !validRunID.MatchString(req.RunID) {
		writeError(w, http.StatusBadRequest, "Invalid benchmark run ID")
		return
	}

	if req.PricePerGBUSD <= 0 || req.PricePerGBUSD > 1_000_000 {
		writeError(w, http.StatusBadRequest, "price_per_gb_usd must be greater than 0 and at most 1000000")
		return
	}

	if req.BatchesPerDay < 1 || req.BatchesPerDay > 1_000_000 {
		writeError(w, http.StatusBadRequest, "benchmark_batches_per_day must be between 1 and 1000000")
		return
	}

	if req.DaysPerMonth < 1 || req.DaysPerMonth > 31 {
		writeError(w, http.StatusBadRequest, "days_per_month must be between 1 and 31")
		return
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Request must contain exactly one JSON object")
		return
	}

	reportPath := filepath.Join(
		a.reportsDir,
		"signalforge-benchmark-"+req.RunID+".json",
	)

	report, err := readReport(reportPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, "Benchmark report not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Unable to read benchmark report")
		return
	}

	if report.RunID != req.RunID {
		writeError(w, http.StatusNotFound, "Benchmark report not found")
		return
	}

	result, err := calculateEstimate(report, req)
	if err != nil {
		writeError(
			w,
			http.StatusUnprocessableEntity,
			err.Error(),
		)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func readReport(path string) (BenchmarkReport, error) {
	var report BenchmarkReport

	data, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return report, err
	}

	if report.RunID == "" || report.SchemaVersion == "" {
		return report, errors.New("missing required report fields")
	}

	return report, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("[signalforge-api] response encoding error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", http.MethodGet)
	writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
}
