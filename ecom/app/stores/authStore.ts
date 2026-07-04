// Auth store: holds the public user object (never the JWT — that lives only in
// the HttpOnly cookie). Session is restored on load via GET /auth/me.

import { create } from "zustand";
import { api, ApiError } from "~/lib/api";
import { tracker } from "~/lib/tracker";
import type { User } from "~/lib/types";

interface AuthState {
  user: User | null;
  status: "idle" | "loading" | "authenticated" | "unauthenticated";
  error: string | null;
  restore: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  signup: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  clearError: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  status: "idle",
  error: null,

  async restore() {
    set({ status: "loading" });
    try {
      const user = await api.me();
      tracker.identify(String(user.id));
      set({ user, status: "authenticated", error: null });
    } catch {
      set({ user: null, status: "unauthenticated" });
    }
  },

  async login(email, password) {
    set({ error: null });
    try {
      const user = await api.login(email, password);
      tracker.identify(String(user.id));
      tracker.lead({ source: "login", user_id: user.id });
      set({ user, status: "authenticated", error: null });
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : "Login failed";
      set({ error: msg });
      throw e;
    }
  },

  async signup(email, password) {
    set({ error: null });
    try {
      const user = await api.signup(email, password);
      tracker.identify(String(user.id));
      tracker.lead({ source: "signup", user_id: user.id });
      set({ user, status: "authenticated", error: null });
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : "Signup failed";
      set({ error: msg });
      throw e;
    }
  },

  async logout() {
    try {
      await api.logout();
    } catch {
      // Ignore — clear local state regardless.
    }
    tracker.identify("");
    set({ user: null, status: "unauthenticated", error: null });
  },

  clearError() {
    set({ error: null });
  },
}));
