
import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

export interface EstimateRequest {
  run_id: string;
  price_per_gb_usd: number;
  benchmark_batches_per_day: number;
  days_per_month: number;
  pricing_profile: 'linear-gb' | 'datadog-logs-ingestion';
  price_per_million_events_usd?: number;
}

export interface MonthlyEstimate {
  original_volume_gb: number;
  filtered_volume_gb: number;
  estimated_volume_saved_gb: number;
  original_cost_usd: number;
  filtered_cost_usd: number;
  estimated_savings_usd: number;
  savings_percent: number;
}

export interface IndexingEstimate {
  pricing_unit: string;
  monthly: {
    original_indexed_events: number;
    filtered_indexed_events: number;
    estimated_events_avoided: number;
    original_cost_usd: number;
    filtered_cost_usd: number;
    estimated_savings_usd: number;
    savings_percent: number;
  };
  disclaimer: string;
}

export interface EstimateResponse {
  run_id: string;
  currency: string;
  pricing_unit: string;
  pricing_profile: string;
  pricing_source_url?: string;
  pricing_note: string;
  assumptions: Record<string, unknown>;
  monthly: MonthlyEstimate;
  indexing?: IndexingEstimate;
  disclaimer: string;
}

@Injectable({ providedIn: 'root' })
export class EstimateService {
  private readonly http = inject(HttpClient);

  createEstimate(request: EstimateRequest): Observable<EstimateResponse> {
    return this.http.post<EstimateResponse>(
      '/api/v1/estimates',
      request
    );
  }
}
