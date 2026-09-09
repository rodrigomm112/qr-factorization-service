import { createRequire } from 'node:module';
import { pino } from 'pino';
import type { DestinationStream, Logger, LoggerOptions } from 'pino';
import type { AppConfig } from './config.js';
import { SERVICE_NAME, SERVICE_VERSION } from './package-info.js';

export type { Logger };

// Bodies are not logged at all (pino-http logs metadata only), so matrices never leave the process.
const REDACTED_PATHS = [
  'req.headers.authorization',
  'req.headers.cookie',
  'req.headers["proxy-authorization"]',
  'res.headers["set-cookie"]',
  'config.jwt.secret',
  '*.secret',
  '*.token',
  '*.password',
];

/** `pino-pretty` is a dev-only dependency; the production image does not ship it. */
function prettyAvailable(): boolean {
  try {
    createRequire(import.meta.url).resolve('pino-pretty');
    return true;
  } catch {
    return false;
  }
}

export function createLogger(config: AppConfig, destination?: DestinationStream): Logger {
  const usePretty =
    destination === undefined &&
    config.appEnv === 'development' &&
    process.stdout.isTTY === true &&
    prettyAvailable();

  const options: LoggerOptions = {
    level: config.logLevel,
    base: { service: SERVICE_NAME, version: SERVICE_VERSION },
    redact: { paths: REDACTED_PATHS, censor: '[redacted]' },
    formatters: {
      level: (label) => ({ level: label }),
    },
    ...(usePretty
      ? {
          transport: {
            target: 'pino-pretty',
            options: { colorize: true, translateTime: 'SYS:HH:MM:ss.l', ignore: 'pid,hostname' },
          },
        }
      : {}),
  };

  return destination === undefined ? pino(options) : pino(options, destination);
}
