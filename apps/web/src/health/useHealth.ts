// `/health/ready` is public and outside the rate limiter, so a probe never costs a token.
// qr-api's readiness also probes stats-api, so its badge can go red while stats-api's stays green.
// The state is lifted here because the header renders it as a badge and the technical details
// render the same document's `version`.
import { useCallback, useEffect, useState } from 'react';
import { fetchReadiness } from '../api/endpoints';
import { ApiError, errorMessage } from '../api/problem';
import type { HealthStatus } from '../api/types';

export const POLL_INTERVAL_MS = 15_000;

export interface HealthTarget {
  readonly name: string;
  readonly baseUrl: string;
}

export type BadgeStatus = HealthStatus | 'unknown';

export interface HealthState {
  readonly status: BadgeStatus;
  /** `1.4.0` once the document arrived; `null` while unknown or on failure. */
  readonly version: string | null;
  /** Human-readable reason, for the badge tooltip. */
  readonly detail: string;
}

export const UNKNOWN_HEALTH: HealthState = {
  status: 'unknown',
  version: null,
  detail: 'sin comprobar',
};

export type HealthStates = Readonly<Record<string, HealthState>>;

export function useHealth(targets: readonly HealthTarget[]): HealthStates {
  const [states, setStates] = useState<HealthStates>({});

  const probe = useCallback(
    async (target: HealthTarget, signal: AbortSignal): Promise<HealthState> => {
      try {
        const health = await fetchReadiness(target.baseUrl, signal);
        return { status: health.status, version: health.version, detail: `v${health.version}` };
      } catch (caught) {
        if (caught instanceof ApiError && caught.kind === 'aborted') return UNKNOWN_HEALTH;
        // A 503 readiness document is not problem+json, so it lands here too.
        return { status: 'fail', version: null, detail: errorMessage(caught) };
      }
    },
    [],
  );

  useEffect(() => {
    const controller = new AbortController();
    let active = true;

    const probeAll = (): void => {
      for (const target of targets) {
        void probe(target, controller.signal).then((state) => {
          if (!active || controller.signal.aborted) return;
          setStates((current) => ({ ...current, [target.name]: state }));
        });
      }
    };

    probeAll();
    const ticker = setInterval(probeAll, POLL_INTERVAL_MS);
    return () => {
      active = false;
      controller.abort();
      clearInterval(ticker);
    };
  }, [targets, probe]);

  return states;
}
