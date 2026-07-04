// Shared domain + API types.

export interface User {
  id: number;
  email: string;
  created_at: string;
}

export interface Product {
  id: number;
  title: string;
  description: string;
  price: number;
  discountPercentage: number;
  rating: number;
  stock: number;
  brand: string;
  category: string;
  thumbnail: string;
  images: string[];
}

export interface ProductsResponse {
  products: Product[];
  total: number;
  skip: number;
  limit: number;
}

export interface CartItem {
  id: number;
  title: string;
  price: number;
  thumbnail: string;
  quantity: number;
}

// --- Stats (mirrors api/internal/model.Stats) ---

export interface TimeBucket {
  bucket: string;
  count: number;
}

export interface TypeTimeBucket {
  bucket: string;
  event_type: string;
  count: number;
}

export interface StageRollup {
  stage_name: string;
  windows: number;
  items_in: number;
  items_out: number;
  dropped: number;
  errors: number;
  batches: number;
  avg_p50_ns: number;
  avg_p99_ns: number;
  avg_throughput: number;
}

export interface Stats {
  avg_capture_ms: number;
  avg_event_params: number;
  total_events: number;
  events_over_time: TimeBucket[] | null;
  type_counts: Record<string, number> | null;
  type_over_time: TypeTimeBucket[] | null;
  stage_rollups: StageRollup[] | null;
}
