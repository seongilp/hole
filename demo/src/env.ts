export interface Env {
  readonly BROWSER: Fetcher;
  readonly SCANS: KVNamespace;
  readonly SCAN_LIMITER: RateLimit;
  readonly ALLOWED_ORIGINS?: string;
}
