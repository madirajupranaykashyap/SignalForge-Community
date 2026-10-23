
import { CommonModule } from '@angular/common';
import { Component } from '@angular/core';

interface DropRule {
  attribute: string;
  value: string;
}

@Component({
  selector: 'app-collector-config',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './collector-config.component.html',
  styleUrl: './collector-config.component.scss',
})
export class CollectorConfigComponent {
  receiverHost = '127.0.0.1';
  grpcPort = 4317;
  httpPort = 4318;

  // One rule per line, formatted as attribute=value.
  dropRulesText = 'http.route=/health\nservice.name=health-check';

  archivePath = './data/archive/logs.jsonl';

  enableOtlpExport = false;
  otlpEndpoint = 'otel-vendor.example:4317';

  copied = false;

  get rules(): DropRule[] {
    return this.dropRulesText
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => {
        const separator = line.indexOf('=');

        return {
          attribute: line.slice(0, separator).trim(),
          value: line.slice(separator + 1).trim(),
        };
      });
  }

  get errors(): string[] {
    const errors: string[] = [];

    const ruleLines = this.dropRulesText
      .split('\n')
      .map((line) => line.trim())
      .filter(Boolean);

    if (!ruleLines.length) {
      errors.push('Add at least one filtering rule.');
    }

    for (const [index, line] of ruleLines.entries()) {
      const separator = line.indexOf('=');

      if (separator <= 0) {
        errors.push(`Rule ${index + 1} must use attribute=value format.`);
        continue;
      }

      if (!line.slice(0, separator).trim()) {
        errors.push(`Rule ${index + 1} needs an attribute name.`);
      }
    }

    if (!this.archivePath.trim()) {
      errors.push('Archive path is required.');
    }

    if (
      !this.receiverHost.trim() ||
      this.receiverHost.includes('\n') ||
      this.receiverHost.includes('\r')
    ) {
      errors.push('Enter a valid receiver host.');
    }

    if (
      !Number.isInteger(this.grpcPort) ||
      this.grpcPort < 1 ||
      this.grpcPort > 65535
    ) {
      errors.push('gRPC port must be between 1 and 65535.');
    }

    if (
      !Number.isInteger(this.httpPort) ||
      this.httpPort < 1 ||
      this.httpPort > 65535
    ) {
      errors.push('HTTP port must be between 1 and 65535.');
    }

    if (
      this.enableOtlpExport &&
      (!this.otlpEndpoint.trim() ||
        !/^[a-zA-Z0-9.-]+:\d{1,5}$/.test(this.otlpEndpoint.trim()))
    ) {
      errors.push('Enter an OTLP gRPC endpoint as hostname:port.');
    }

    return errors;
  }

  get yaml(): string {
    if (this.errors.length) {
      return '# Fix the validation errors to generate a configuration.\n';
    }

    const expressions = this.rules.map(
      (rule) =>
        `          - ${JSON.stringify(
          `attributes[${JSON.stringify(rule.attribute)}] == ${JSON.stringify(rule.value)}`,
        )}`,
    );

    const vendorExporter = this.enableOtlpExport
      ? `  otlp/vendor:
    endpoint: ${JSON.stringify(this.otlpEndpoint.trim())}`
      : '';

    const forwardingExporters = this.enableOtlpExport
      ? '[debug, otlp/vendor]'
      : '[debug]';

    return `# SignalForge-generated OpenTelemetry Collector configuration
# Review this file before deploying it to production.
# Raw logs are archived locally. Only the filtered pipeline uses the
# optional OTLP destination.

receivers:
  otlp:
    protocols:
      grpc:
        endpoint: ${JSON.stringify(`${this.receiverHost.trim()}:${this.grpcPort}`)}
      http:
        endpoint: ${JSON.stringify(`${this.receiverHost.trim()}:${this.httpPort}`)}

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 256
  batch:
    timeout: 5s
  filter/drop-noisy:
    error_mode: ignore
    logs:
      log_record:
${expressions.join('\n')}

exporters:
  debug:
    verbosity: basic
  file/archive:
    path: ${JSON.stringify(this.archivePath.trim())}
    append: true
    create_directory: true
${vendorExporter}
service:
  pipelines:
    # Filtered pipeline: only non-matching records continue to exporters.
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch, filter/drop-noisy]
      exporters: ${forwardingExporters}

    # Archive pipeline: receives the original records before filtering.
    logs/archive:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [file/archive]
`;
  }

  get yamlLineCount(): number {
    return this.yaml.split('\n').length;
  }

  onRulesChange(event: Event): void {
    this.dropRulesText = (event.target as HTMLTextAreaElement).value;
    this.copied = false;
  }

  onArchivePathChange(event: Event): void {
    this.archivePath = (event.target as HTMLInputElement).value;
    this.copied = false;
  }

  onReceiverHostChange(event: Event): void {
    this.receiverHost = (event.target as HTMLInputElement).value;
    this.copied = false;
  }

  onGrpcPortChange(event: Event): void {
    this.grpcPort = Number((event.target as HTMLInputElement).value);
    this.copied = false;
  }

  onHttpPortChange(event: Event): void {
    this.httpPort = Number((event.target as HTMLInputElement).value);
    this.copied = false;
  }

  onOtlpEndpointChange(event: Event): void {
    this.otlpEndpoint = (event.target as HTMLInputElement).value;
    this.copied = false;
  }

  onOtlpToggle(event: Event): void {
    this.enableOtlpExport = (event.target as HTMLInputElement).checked;
    this.copied = false;
  }

  async copyYaml(): Promise<void> {
    if (this.errors.length) return;

    await navigator.clipboard.writeText(this.yaml);
    this.copied = true;
  }

  downloadYaml(): void {
    if (this.errors.length) return;

    const blob = new Blob([this.yaml], {
      type: 'application/yaml;charset=utf-8',
    });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');

    anchor.href = url;
    anchor.download = 'signalforge-collector.yaml';
    anchor.click();

    URL.revokeObjectURL(url);
  }
}
