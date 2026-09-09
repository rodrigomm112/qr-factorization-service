import { ApiError } from "../api/problem";

export interface ProblemAlertProps {
  readonly error: Error;
  readonly title?: string;
}

/** Mapped message, the server's `title`/`status`/`detail`, `errors[]`, then the `requestId`. */
export function ProblemAlert({ error, title }: ProblemAlertProps) {
  const apiError = error instanceof ApiError ? error : null;
  const problem = apiError?.problem;

  return (
    <div className="alert" role="alert">
      <p className="alert-headline">
        {title !== undefined && <strong>{title} </strong>}
        {error.message}
      </p>
      {problem !== undefined && (
        <>
          <p className="alert-detail">
            <span className="badge">{problem.status}</span>
            <span className="mono">{problem.title}</span>
            {problem.detail !== undefined && <> — {problem.detail}</>}
          </p>
          {problem.errors !== undefined && problem.errors.length > 0 && (
            <ul className="issues">
              {problem.errors.map((issue) => (
                <li key={`${issue.pointer}:${issue.code}`}>
                  <code>{issue.pointer}</code>{" "}
                  <span className="code">{issue.code}</span> {issue.message}
                </li>
              ))}
            </ul>
          )}
        </>
      )}
      {apiError?.requestId !== undefined && (
        <p className="alert-meta mono">requestId: {apiError.requestId}</p>
      )}
    </div>
  );
}
