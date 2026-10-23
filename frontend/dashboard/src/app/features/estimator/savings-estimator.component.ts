
import { CommonModule } from '@angular/common';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import {
  BenchmarkReport
} from '../../core/services/benchmark.service';
import { BenchmarkService } from '../../core/services/benchmark.service';
import {
  EstimateRequest,
  EstimateResponse,
  EstimateService
} from '../../core/services/estimate.service';

@Component({
  selector: 'app-savings-estimator',
  standalone: true,
  imports: [CommonModule, FormsModule],
  templateUrl: './savings-estimator.component.html',
  styleUrl: './savings-estimator.component.scss'
})
export class SavingsEstimatorComponent implements OnInit {
  private readonly benchmarkService = inject(BenchmarkService);
  private readonly estimateService = inject(EstimateService);

  readonly reports = signal<BenchmarkReport[]>([]);
  readonly estimate = signal<EstimateResponse | null>(null);
  readonly loadingReports = signal(true);
  readonly calculating = signal(false);
  readonly error = signal('');

  selectedRunId = '';
  pricingProfile: 'linear-gb' | 'datadog-logs-ingestion' = 'linear-gb';
  pricePerGB = 0.10;
  batchesPerDay = 1000;
  daysPerMonth = 30;
  pricePerMillionEvents: number | null = null;

  ngOnInit(): void {
    this.benchmarkService.getBenchmarks().subscribe({
      next: (reports) => {
        const sorted = [...reports].sort((a, b) =>
          Date.parse(b.generated_at_utc ?? '') -
          Date.parse(a.generated_at_utc ?? '')
        );

        this.reports.set(sorted);
        this.selectedRunId = sorted[0]?.run_id ?? '';
        this.loadingReports.set(false);
      },
      error: () => {
        this.error.set('Could not load benchmark runs. Check that the Go API is running.');
        this.loadingReports.set(false);
      }
    });
  }

  calculate(): void {
    if (!this.selectedRunId) {
      this.error.set('Select a benchmark run first.');
      return;
    }

    if (
      !Number.isFinite(this.pricePerGB) ||
      this.pricePerGB < 0 ||
      this.pricePerGB > 1_000_000 ||
      !Number.isInteger(this.batchesPerDay) ||
      this.batchesPerDay < 1 ||
      this.batchesPerDay > 1_000_000 ||
      !Number.isInteger(this.daysPerMonth) ||
      this.daysPerMonth < 1 ||
      this.daysPerMonth > 31 ||
      (this.pricePerMillionEvents !== null &&
        (!Number.isFinite(this.pricePerMillionEvents) ||
          this.pricePerMillionEvents < 0 ||
          this.pricePerMillionEvents > 1_000_000))
    ) {
      this.error.set('Check your pricing and monthly volume inputs.');
      return;
    }

    if (
      this.pricingProfile === 'datadog-logs-ingestion' &&
      this.pricePerGB !== 0 &&
      this.pricePerGB !== 0.10
    ) {
      this.error.set(
        'The Datadog profile currently supports its configured default rate of $0.10/GB. Use $0.10 or select Generic per GB.'
      );
      return;
    }

    const request: EstimateRequest = {
      run_id: this.selectedRunId,
      price_per_gb_usd: this.pricePerGB,
      benchmark_batches_per_day: this.batchesPerDay,
      days_per_month: this.daysPerMonth,
      pricing_profile: this.pricingProfile
    };

    if (
      this.pricePerMillionEvents !== null &&
      this.pricePerMillionEvents > 0
    ) {
      request.price_per_million_events_usd = this.pricePerMillionEvents;
    }

    this.calculating.set(true);
    this.error.set('');
    this.estimate.set(null);

    this.estimateService.createEstimate(request).subscribe({
      next: (result) => {
        this.estimate.set(result);
        this.calculating.set(false);
      },
      error: (err) => {
        this.error.set(
          err?.error?.error ??
          'Could not calculate the estimate. Check the API response and your inputs.'
        );
        this.calculating.set(false);
      }
    });
  }

  formatCurrency(value: number | undefined): string {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD'
    }).format(value ?? 0);
  }

  formatNumber(value: number | undefined): string {
    return new Intl.NumberFormat('en-US', {
      maximumFractionDigits: 2
    }).format(value ?? 0);
  }

  get selectedReport(): BenchmarkReport | undefined {
    return this.reports().find(r => r.run_id === this.selectedRunId);
  }
}
