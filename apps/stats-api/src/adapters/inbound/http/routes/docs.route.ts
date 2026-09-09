import { Router } from 'express';
import type { NextFunction, Request, Response } from 'express';
import swaggerUi from 'swagger-ui-express';
import { OPENAPI_PATH, SERVICE_VERSION } from '../../../../platform/package-info.js';
import { methodNotAllowed } from '../middleware/not-found.middleware.js';

const SWAGGER_OPTIONS = {
  // Fetched from this service, so `/docs` always reflects `openapi.yaml` on disk.
  swaggerOptions: { url: '/openapi.yaml' },
  customSiteTitle: `stats-api ${SERVICE_VERSION} — API reference`,
  customCss: '.swagger-ui .topbar { display: none }',
};

/** Swagger UI comes from the bundled `swagger-ui-dist`, never a CDN: works offline, tight CSP. */
export function docsRouter(): Router {
  const router = Router();

  router.get('/openapi.yaml', (_req: Request, res: Response, next: NextFunction): void => {
    res.setHeader('Content-Type', 'application/yaml');
    res.sendFile(OPENAPI_PATH, { headers: { 'Content-Type': 'application/yaml' } }, (error) => {
      if (error) next(error);
    });
  });
  router.all('/openapi.yaml', methodNotAllowed(['GET', 'HEAD']));

  router.use('/docs', swaggerUi.serveFiles(undefined, SWAGGER_OPTIONS), swaggerUi.setup(undefined, SWAGGER_OPTIONS));

  return router;
}
