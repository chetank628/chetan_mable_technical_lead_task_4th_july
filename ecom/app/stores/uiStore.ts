// UI store: consent + transient UI flags (toasts). Consent is persisted so the
// banner doesn't reappear every load. Tracking only sends consent:true after
// the user accepts.

import { create } from "zustand";
import { persist } from "zustand/middleware";

export type Toast = {
  id: number;
  message: string;
  kind: "success" | "error" | "info";
};

interface UiState {
  consent: "unset" | "granted" | "denied";
  toasts: Toast[];
  setConsent: (granted: boolean) => void;
  pushToast: (message: string, kind?: Toast["kind"]) => void;
  dismissToast: (id: number) => void;
}

let toastId = 0;

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      consent: "unset",
      toasts: [],

      setConsent(granted) {
        set({ consent: granted ? "granted" : "denied" });
      },

      pushToast(message, kind = "info") {
        const id = ++toastId;
        set((state) => ({ toasts: [...state.toasts, { id, message, kind }] }));
        // Auto-dismiss after 4s.
        setTimeout(() => {
          set((state) => ({
            toasts: state.toasts.filter((t) => t.id !== id),
          }));
        }, 4000);
      },

      dismissToast(id) {
        set((state) => ({ toasts: state.toasts.filter((t) => t.id !== id) }));
      },
    }),
    {
      name: "mable_ui",
      // Only persist consent, not transient toasts.
      partialize: (state) => ({ consent: state.consent }),
    }
  )
);
