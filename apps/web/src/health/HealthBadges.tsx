import { UNKNOWN_HEALTH } from './useHealth';
import type { BadgeStatus, HealthStates, HealthTarget } from './useHealth';

/** The header only says whether the API answers; the version hash lives in the tooltip. */
const LABEL: Record<BadgeStatus, string> = {
  pass: 'operativo',
  warn: 'degradado',
  fail: 'sin respuesta',
  unknown: 'sin respuesta',
};

export interface HealthBadgesProps {
  readonly targets: readonly HealthTarget[];
  readonly states: HealthStates;
}

export function HealthBadges({ targets, states }: HealthBadgesProps) {
  return (
    <ul className="health" aria-live="polite" aria-label="Estado de las APIs">
      {targets.map((target) => {
        const state = states[target.name] ?? UNKNOWN_HEALTH;
        return (
          <li
            key={target.name}
            className={`health-badge ${state.status}`}
            title={`${target.baseUrl} · ${state.detail}`}
          >
            <span className="dot" aria-hidden="true" />
            <span className="health-name">{target.name}</span>
            <span className="health-status">{LABEL[state.status]}</span>
          </li>
        );
      })}
    </ul>
  );
}
