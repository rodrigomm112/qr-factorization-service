// Dev default. Rewritten in the container by docker-entrypoint.d/10-app-config.sh
// from QR_API_BASE_URL / STATS_API_BASE_URL.
window.__APP_CONFIG__ = {
  qrApiBaseUrl: 'http://localhost:8080',
  statsApiBaseUrl: 'http://localhost:3000',
};
