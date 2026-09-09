import { useEffect, useRef, useState } from 'react';

/** How long the button reads «Copiado» before falling back to «Copiar». */
export const COPIED_FEEDBACK_MS = 1_500;

// `navigator.clipboard` is typed as always present, but it is absent outside a secure context
// (plain http on a LAN address) and in jsdom, hence the widened lookup.
const clipboardApi = (): Clipboard | undefined =>
  (globalThis as { navigator?: Navigator }).navigator?.clipboard;

export const clipboardAvailable = (): boolean => typeof clipboardApi()?.writeText === 'function';

export interface CopyButtonProps {
  /** Already-serialized payload: the caller decides what "this block" means. */
  readonly text: string;
  /** Accessible name, e.g. `Copiar Q en JSON`. The visible label stays «Copiar». */
  readonly label: string;
}

/** Renders nothing where the clipboard is unavailable: a button that cannot work is noise. */
export function CopyButton({ text, label }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current !== null) clearTimeout(timer.current);
    },
    [],
  );

  const clipboard = clipboardApi();
  if (clipboard === undefined || typeof clipboard.writeText !== 'function') return null;

  const handleClick = (): void => {
    void clipboard.writeText(text).then(
      () => {
        setCopied(true);
        if (timer.current !== null) clearTimeout(timer.current);
        timer.current = setTimeout(() => {
          setCopied(false);
        }, COPIED_FEEDBACK_MS);
      },
      () => {
        // A denied permission is not an application error: leave the label alone.
        setCopied(false);
      },
    );
  };

  return (
    <button type="button" className="copy-button" aria-label={label} onClick={handleClick}>
      {copied ? 'Copiado' : 'Copiar'}
    </button>
  );
}
