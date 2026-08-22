import {
  createContext,
  useCallback,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button, Sheet } from "./ui";

// Replaces window.confirm.
//
// Browsers suppress native dialogs — Chrome offers "prevent this page from
// creating additional dialogs" after a few, and mobile browsers block them in
// several situations. A suppressed confirm() returns false, so the action it
// guarded silently does nothing and the app looks broken with no error to
// explain it. An in-app dialog cannot be switched off by the browser, and it
// can say what is about to be deleted.

interface ConfirmRequest {
  title: string;
  message?: string;
  confirmLabel?: string;
  isDestructive?: boolean;
}

type ConfirmFunction = (request: ConfirmRequest) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFunction | undefined>(undefined);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [request, setRequest] = useState<ConfirmRequest | null>(null);
  // Held in a ref so resolving does not depend on a re-render having happened.
  const resolveRef = useRef<((answer: boolean) => void) | null>(null);

  const confirm = useCallback<ConfirmFunction>((nextRequest) => {
    setRequest(nextRequest);
    return new Promise<boolean>((resolve) => {
      resolveRef.current = resolve;
    });
  }, []);

  const settle = (answer: boolean) => {
    resolveRef.current?.(answer);
    resolveRef.current = null;
    setRequest(null);
  };

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      <Sheet
        title={request?.title ?? ""}
        isOpen={request !== null}
        onClose={() => settle(false)}
      >
        {request?.message && (
          <p className="mb-4 text-sm text-ink-soft">{request.message}</p>
        )}
        <div className="flex gap-2">
          <Button
            variant={request?.isDestructive === false ? "primary" : "danger"}
            className="flex-1"
            onClick={() => settle(true)}
          >
            {request?.confirmLabel ?? "Delete"}
          </Button>
          <Button variant="ghost" className="flex-1" onClick={() => settle(false)}>
            Cancel
          </Button>
        </div>
      </Sheet>
    </ConfirmContext.Provider>
  );
}

export function useConfirm(): ConfirmFunction {
  const confirm = useContext(ConfirmContext);
  if (!confirm) {
    throw new Error("useConfirm must be used inside a ConfirmProvider");
  }
  return confirm;
}
