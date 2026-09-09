import { randomUUID } from 'node:crypto';
import express from 'express';
import type { Express } from 'express';
import { pinoHttp } from 'pino-http';
import type { AppConfig } from '../../../platform/config.js';
import type { Logger } from '../../../platform/logger.js';
import { errorMiddleware } from './middleware/error.middleware.js';
import { notFoundMiddleware } from './middleware/not-found.middleware.js';
import { requestIdMiddleware, REQUEST_ID_HEADER } from './middleware/request-id.middleware.js';
import {
  contentTypeMiddleware,
  corsMiddleware,
  rateLimitMiddleware,
  securityHeadersMiddleware,
} from './middleware/security.middleware.js';
import { docsRouter } from './routes/docs.route.js';
import { healthRouter } from './routes/health.route.js';
import { statisticsRouter } from './routes/statistics.route.js';

export interface AppDependencies {
  readonly config: AppConfig;
  readonly logger: Logger;
}

const API_PREFIX = '/api/v1';

// No socket is bound, so the integration suite drives the same chain through supertest. The order
// is the contract: correlation id, logging, security, parsing, routes, then the terminal 404 and
// error renderer.
export function buildApp({ config, logger }: AppDependencies): Express {
  const app = express();
  const production = config.appEnv === 'production';

  app.disable('x-powered-by');
  if (config.trustProxy) {
    // Exactly one hop: the container's reverse proxy / Cloud Run front end.
    app.set('trust proxy', 1);
  }

  app.use(requestIdMiddleware());

  app.use(
    pinoHttp({
      logger,
      genReqId: (_req, res) => {
        const header = res.getHeader(REQUEST_ID_HEADER);
        return typeof header === 'string' ? header : randomUUID();
      },
      customLogLevel: (_req, res, error) => {
        if (error !== undefined || res.statusCode >= 500) return 'error';
        return res.statusCode >= 400 ? 'warn' : 'info';
      },
      // Probes run every few seconds and would drown the useful lines.
      autoLogging: { ignore: (req) => req.url?.startsWith('/health') === true },
    }),
  );

  app.use(securityHeadersMiddleware(production));
  app.use(corsMiddleware(config.corsAllowedOrigins));

  app.use(API_PREFIX, rateLimitMiddleware(config.rateLimit));
  app.use(API_PREFIX, contentTypeMiddleware());
  app.use(express.json({ limit: config.maxBodyBytes, strict: true, type: 'application/json' }));

  app.use(API_PREFIX, statisticsRouter(config));
  app.use('/health', healthRouter());
  app.use(docsRouter());

  app.use(notFoundMiddleware());
  app.use(errorMiddleware(logger, config.maxBodyBytes));

  return app;
}
