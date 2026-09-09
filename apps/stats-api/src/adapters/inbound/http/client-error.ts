import { randomUUID } from 'node:crypto';
import { STATUS_CODES, type Server } from 'node:http';
import type { Socket } from 'node:net';
import type { Logger } from '../../../platform/logger.js';
import { PROBLEM_REGISTRY, PROBLEM_TYPE_PREFIX, type ProblemSlug } from './problem.js';

// Node's parser rejects a header block over `server.maxHeaderSize` (16 KiB) or a malformed request
// line before any middleware runs, and answers with a bare status line. This hook writes the
// contract's problem document onto the socket instead.
const PARSER_PROBLEMS: Readonly<Record<string, ProblemSlug>> = {
  HPE_HEADER_OVERFLOW: 'request-header-fields-too-large',
};

/** `POST /api/v1/statistics?x=1 HTTP/1.1` -> `/api/v1/statistics`. */
function requestTargetOf(rawPacket: Buffer | undefined): string {
  const firstLine = rawPacket?.toString('latin1').split('\r\n')[0] ?? '';
  const target = firstLine.split(' ')[1];
  if (target === undefined || !target.startsWith('/')) return '/';
  return target.split('?')[0] ?? '/';
}

function problemPacket(slug: ProblemSlug, instance: string): string {
  const definition = PROBLEM_REGISTRY[slug];
  const body = JSON.stringify({
    type: `${PROBLEM_TYPE_PREFIX}${slug}`,
    title: definition.title,
    status: definition.status,
    detail: definition.detail,
    instance,
    requestId: randomUUID(),
    timestamp: new Date().toISOString(),
  });
  const payload = Buffer.from(body, 'utf8');
  return [
    // Canonical reason phrase in the status line, not the registry `title`.
    `HTTP/1.1 ${String(definition.status)} ${STATUS_CODES[definition.status] ?? ''}`,
    'Content-Type: application/problem+json',
    `Content-Length: ${String(payload.byteLength)}`,
    'Connection: close',
    '',
    body,
  ].join('\r\n');
}

/** Anything the registry does not cover falls through to Node, which closes the connection. */
export function installClientErrorHandler(server: Server, logger: Logger): void {
  server.on('clientError', (error: NodeJS.ErrnoException & { rawPacket?: Buffer }, socket: Socket) => {
    const slug = PARSER_PROBLEMS[error.code ?? ''];
    if (slug === undefined || socket.destroyed || !socket.writable) {
      socket.destroy(error);
      return;
    }
    // A rejected request is the client's mistake, like any 4xx the app renders: warn, never error.
    logger.warn({ code: error.code }, 'rejected a request during header parsing');
    socket.end(problemPacket(slug, requestTargetOf(error.rawPacket)));
  });
}
