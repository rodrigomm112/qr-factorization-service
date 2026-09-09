import { Router } from 'express';
import type { Request, Response } from 'express';
import { computeStatistics } from '../../../../application/compute-statistics.usecase.js';
import type { AppConfig } from '../../../../platform/config.js';
import { jwtMiddleware } from '../middleware/jwt.middleware.js';
import { methodNotAllowed } from '../middleware/not-found.middleware.js';
import { requestIdOf } from '../middleware/request-id.middleware.js';
import { hasBody } from '../middleware/security.middleware.js';
import { ProblemError } from '../problem.js';
import { parseStatisticsRequest } from '../schemas/statistics.schema.js';

export function statisticsRouter(config: AppConfig): Router {
  const router = Router();

  router.post(
    '/statistics',
    jwtMiddleware(config.jwt),
    (req: Request, res: Response): void => {
      // An empty payload is not a JSON document, so it is a syntax failure, not a missing member.
      if (!hasBody(req)) {
        throw new ProblemError('malformed-json');
      }
      const body: unknown = req.body;
      const matrices = parseStatisticsRequest(body, config.limits);
      const result = computeStatistics(matrices, { tolerance: config.diagonalTolerance });
      res.status(200).json({ requestId: requestIdOf(res), ...result });
    },
  );

  router.all('/statistics', methodNotAllowed(['POST']));

  return router;
}
