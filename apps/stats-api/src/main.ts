import type { Server } from 'node:http';
import { buildApp } from './adapters/inbound/http/app.js';
import { installClientErrorHandler } from './adapters/inbound/http/client-error.js';
import { ConfigurationError, loadConfig } from './platform/config.js';
import { createLogger } from './platform/logger.js';
import type { Logger } from './platform/logger.js';
import { SERVICE_NAME } from './platform/package-info.js';

function fail(message: string): never {
  process.stderr.write(`${message}\n`);
  process.exit(1);
}

/** Stops accepting connections, drains in-flight requests, then hard-exits on deadline. */
function installShutdownHandlers(server: Server, logger: Logger, timeoutMs: number): void {
  let closing = false;

  const shutdown = (signal: string): void => {
    if (closing) return;
    closing = true;
    logger.info({ signal }, 'shutdown requested, draining connections');

    const timer = setTimeout(() => {
      logger.error({ timeoutMs }, 'graceful shutdown timed out, forcing exit');
      process.exit(1);
    }, timeoutMs);
    timer.unref();

    server.close((error) => {
      clearTimeout(timer);
      if (error) {
        logger.error({ err: error }, 'error while closing the server');
        process.exit(1);
      }
      logger.info('shutdown complete');
      process.exit(0);
    });
    server.closeIdleConnections();
  };

  process.on('SIGTERM', () => {
    shutdown('SIGTERM');
  });
  process.on('SIGINT', () => {
    shutdown('SIGINT');
  });

  process.on('unhandledRejection', (reason) => {
    logger.fatal({ err: reason }, 'unhandled promise rejection');
    process.exit(1);
  });
  process.on('uncaughtException', (error) => {
    logger.fatal({ err: error }, 'uncaught exception');
    process.exit(1);
  });
}

function main(): void {
  let config;
  try {
    config = loadConfig();
  } catch (error) {
    if (error instanceof ConfigurationError) fail(error.message);
    throw error;
  }

  const logger = createLogger(config);
  const app = buildApp({ config, logger });

  const server = app.listen(config.port, () => {
    // `service` and `version` are already part of every line's base fields.
    logger.info({ port: config.port, appEnv: config.appEnv }, `${SERVICE_NAME} listening`);
  });

  // A header block above server.maxHeaderSize (16 KiB) never reaches Express, so the contract's 431
  // is written on the raw socket. The limit stays at Node's default: it matches qr-api's buffer.
  installClientErrorHandler(server, logger);

  // Above the platform's idle timeout so a proxy never reuses a connection about to be dropped.
  server.headersTimeout = 20_000;
  server.requestTimeout = 30_000;
  server.keepAliveTimeout = 65_000;

  installShutdownHandlers(server, logger, config.shutdownTimeoutMs);
}

main();
