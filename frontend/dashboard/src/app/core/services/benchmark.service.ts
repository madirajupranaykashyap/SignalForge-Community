
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, map } from 'rxjs';

export interface BenchmarkResults {
  records_submitted?: number;
  records_received?: number;
  records_filtered?: number;
  records_sent_to_debug_exporter?: number;
  records_archived?: number;
  record_reduction_percent?: number;
  original_otlp_json_bytes?: number;
  filtered_otlp_json_equivalent_bytes?: number;
  estimated_bytes_saved?: number;
  estimated_byte_reduction_percent?: number;
  log_events_submitted?: number;
  log_events_filtered?: number;
  log_events_sent_to_debug_exporter?: number;
  log_events_archived?: number;
}

export interface BenchmarkReport {
  run_id: string;
  generated_at_utc?: string;
  results: BenchmarkResults;
}

@Injectable({ providedIn: 'root' })
export class BenchmarkService {
  private readonly http = inject(HttpClient);

  getBenchmarks(): Observable<BenchmarkReport[]> {
    return this.http
      .get<unknown>('/api/v1/benchmarks')
      .pipe(map((response) => this.normalizeReports(response)));
  }

  private normalizeReports(response: unknown): BenchmarkReport[] {
    let items: unknown = response;

    if (!Array.isArray(items) && typeof items === 'object' && items !== null) {
      const wrapper = items as Record<string, unknown>;
      items = wrapper['benchmarks'] ?? wrapper['reports'] ?? [];
    }

    if (!Array.isArray(items)) {
      throw new Error('Unexpected benchmark API response format.');
    }

    return items.filter(
      (item): item is BenchmarkReport =>
        typeof item === 'object' &&
        item !== null &&
        typeof (item as BenchmarkReport).run_id === 'string' &&
        typeof (item as BenchmarkReport).results === 'object' &&
        (item as BenchmarkReport).results !== null
    );
  }
}
