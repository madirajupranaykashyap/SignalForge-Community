
import {
  ChangeDetectionStrategy,
  Component,
  OnInit,
  computed,
  inject,
  signal
} from '@angular/core';
import {
  BenchmarkReport,
  BenchmarkService
} from './core/services/benchmark.service';

import { SavingsEstimatorComponent } from
  './features/estimator/savings-estimator.component';
import { CollectorConfigComponent } from './features/collector-config/collector-config.component';

type DashboardStatus = 'loading' | 'ready' | 'empty' | 'error';

@Component({
  selector: 'app-root',
  standalone: true,
  templateUrl: './app.html',
  styleUrl: './app.scss',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    SavingsEstimatorComponent,
    CollectorConfigComponent
  ]
})
export class App implements OnInit {
  private readonly benchmarkService = inject(BenchmarkService);

  readonly reports = signal<BenchmarkReport[]>([]);
  readonly status = signal<DashboardStatus>('loading');
  readonly errorMessage = signal('');

  readonly latest = computed(() => this.reports()[0] ?? null);

  ngOnInit(): void {
    this.refresh();
  }

  refresh(): void {
    this.status.set('loading');
    this.errorMessage.set('');

    this.benchmarkService.getBenchmarks().subscribe({
      next: (reports) => {
        const sorted = [...reports].sort((a, b) => {
          const aTime = Date.parse(a.generated_at_utc ?? '') || 0;
          const bTime = Date.parse(b.generated_at_utc ?? '') || 0;
          return bTime - aTime;
        });

        this.reports.set(sorted);
        this.status.set(sorted.length ? 'ready' : 'empty');
      },
      error: () => {
        this.status.set('error');
        this.errorMessage.set(
          'Could not load benchmarks. Check that the Go API is running and the Angular proxy is configured.'
        );
      }
    });
  }

  eventCount(report: BenchmarkReport): number {
    return report.results.log_events_submitted
      ?? report.results.records_submitted
      ?? 0;
  }

  filteredCount(report: BenchmarkReport): number {
    return report.results.log_events_filtered
      ?? report.results.records_filtered
      ?? 0;
  }

  forwardedCount(report: BenchmarkReport): number {
    return report.results.log_events_sent_to_debug_exporter
      ?? report.results.records_sent_to_debug_exporter
      ?? 0;
  }

  archivedCount(report: BenchmarkReport): number {
    return report.results.log_events_archived
      ?? report.results.records_archived
      ?? 0;
  }

  filteredPercent(report: BenchmarkReport): number {
    const submitted = this.eventCount(report);
    const reported = report.results.record_reduction_percent;

    if (reported !== undefined) {
      return reported;
    }

    return submitted > 0
      ? (this.filteredCount(report) / submitted) * 100
      : 0;
  }

  payloadReduction(report: BenchmarkReport): number {
    return report.results.estimated_byte_reduction_percent ?? 0;
  }

  bytesSaved(report: BenchmarkReport): number {
    return report.results.estimated_bytes_saved ?? 0;
  }

  archiveCompleteness(report: BenchmarkReport): number {
    const submitted = this.eventCount(report);

    return submitted > 0
      ? Math.min(100, (this.archivedCount(report) / submitted) * 100)
      : 0;
  }

  originalBytes(report: BenchmarkReport): number {
    return report.results.original_otlp_json_bytes ?? 0;
  }

  filteredBytes(report: BenchmarkReport): number {
    return report.results.filtered_otlp_json_equivalent_bytes ?? 0;
  }

  barWidth(value: number, report: BenchmarkReport): number {
    const max = Math.max(this.originalBytes(report), 1);
    return Math.min(100, (value / max) * 100);
  }

  formatBytes(bytes: number): string {
    if (bytes < 1000) {
      return `${bytes} B`;
    }

    if (bytes < 1_000_000) {
      return `${(bytes / 1000).toFixed(1)} KB`;
    }

    return `${(bytes / 1_000_000).toFixed(2)} MB`;
  }

  formatNumber(value: number): string {
    return new Intl.NumberFormat('en-US', {
      maximumFractionDigits: 0
    }).format(value);
  }

  formatPercent(value: number): string {
    return `${value.toFixed(2)}%`;
  }

  formatDate(value?: string): string {
    if (!value) {
      return '—';
    }

    const date = new Date(value);

    return Number.isNaN(date.getTime())
      ? value
      : date.toLocaleString();
  }
}
