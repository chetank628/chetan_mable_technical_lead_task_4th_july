// Central API client. Every request is credentialed (cookie JWT) to match the
// API's CORS + HttpOnly-cookie auth. The session token is never read by JS.

import type { Product, ProductsResponse, Stats, User } from "./types";

const API_BASE = import.meta.env.VITE_API_BASE || "http://localhost:8080";
const DUMMYJSON = "https://dummyjson.com";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, {
      ...init,
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        ...(init?.headers || {}),
      },
    });
  } catch (e) {
    // Network failure / API down.
    throw new ApiError("Network error — is the API running?", 0);
  }

  let body: unknown = null;
  const text = await res.text();
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }

  if (!res.ok) {
    const message =
      (body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `Request failed (${res.status})`) || `Request failed (${res.status})`;
    throw new ApiError(message, res.status);
  }
  return body as T;
}

// --- Auth ---

export const api = {
  signup(email: string, password: string): Promise<User> {
    return request<User>("/api/auth/signup", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
  },

  login(email: string, password: string): Promise<User> {
    return request<User>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
  },

  logout(): Promise<{ status: string }> {
    return request<{ status: string }>("/api/auth/logout", { method: "POST" });
  },

  me(): Promise<User> {
    return request<User>("/api/auth/me");
  },

  stats(params?: {
    since?: string;
    until?: string;
    granularity?: string;
  }): Promise<Stats> {
    const q = new URLSearchParams();
    if (params?.since) q.set("since", params.since);
    if (params?.until) q.set("until", params.until);
    if (params?.granularity) q.set("granularity", params.granularity);
    const qs = q.toString();
    return request<Stats>(`/api/stats${qs ? `?${qs}` : ""}`);
  },
};

// --- Product catalog (DummyJSON, external; not credentialed) ---

export async function fetchProducts(limit = 30): Promise<Product[]> {
  const res = await fetch(`${DUMMYJSON}/products?limit=${limit}`);
  if (!res.ok) throw new ApiError("Failed to load products", res.status);
  const data = (await res.json()) as ProductsResponse;
  return data.products;
}

export async function fetchProduct(id: string | number): Promise<Product> {
  const res = await fetch(`${DUMMYJSON}/products/${id}`);
  if (!res.ok) throw new ApiError("Product not found", res.status);
  return (await res.json()) as Product;
}

export async function fetchCategories(): Promise<string[]> {
  try {
    const res = await fetch(`${DUMMYJSON}/products/category-list`);
    if (!res.ok) return [];
    return (await res.json()) as string[];
  } catch {
    return [];
  }
}
