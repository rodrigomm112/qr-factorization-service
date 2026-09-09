import { Router } from 'express';
import type { Request, Response } from 'express';
import { SERVICE_NAME, SERVICE_VERSION } from '../../../../platform/package-info.js';
import { methodNotAllowed } from '../middleware/not-found.middleware.js';

const HEALTH_MEDIA_TYPE = 'application/health+json';

function respond(_req: Request, res: Response): void {
  const body = {
    status: 'pass',
    service: SERVICE_NAME,
    version: SERVICE_VERSION,
    uptimeSeconds: Math.round(process.uptime() * 1000) / 1000,
  };
  // Buffer, not string: `res.send` would append `; charset=utf-8` to the media type.
  res.setHeader('Content-Type', HEALTH_MEDIA_TYPE);
  res.status(200).send(Buffer.from(JSON.stringify(body), 'utf8'));
}

// No downstream dependency, so readiness is liveness; the endpoints exist because the compose
// healthcheck, Cloud Run and qr-api's readiness probe all target them.
export function healthRouter(): Router {
  const router = Router();

  router.get('/live', respond);
  router.all('/live', methodNotAllowed(['GET', 'HEAD']));

  router.get('/ready', respond);
  router.all('/ready', methodNotAllowed(['GET', 'HEAD']));

  return router;
}
