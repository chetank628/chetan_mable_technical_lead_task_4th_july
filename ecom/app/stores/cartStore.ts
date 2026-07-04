// Cart store: persisted to localStorage (NOT tied to auth, so it survives login,
// logout, refresh, and session expiry). This satisfies the "cart preserved on
// 401 / refresh mid-checkout" edge cases.

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { CartItem, Product } from "~/lib/types";

interface CartState {
  items: CartItem[];
  addItem: (product: Product, quantity?: number) => void;
  removeItem: (id: number) => void;
  setQuantity: (id: number, quantity: number) => void;
  clear: () => void;
  count: () => number;
  subtotal: () => number;
}

export const useCartStore = create<CartState>()(
  persist(
    (set, get) => ({
      items: [],

      addItem(product, quantity = 1) {
        set((state) => {
          const existing = state.items.find((i) => i.id === product.id);
          if (existing) {
            return {
              items: state.items.map((i) =>
                i.id === product.id
                  ? { ...i, quantity: i.quantity + quantity }
                  : i
              ),
            };
          }
          return {
            items: [
              ...state.items,
              {
                id: product.id,
                title: product.title,
                price: product.price,
                thumbnail: product.thumbnail,
                quantity,
              },
            ],
          };
        });
      },

      removeItem(id) {
        set((state) => ({ items: state.items.filter((i) => i.id !== id) }));
      },

      setQuantity(id, quantity) {
        if (quantity <= 0) {
          get().removeItem(id);
          return;
        }
        set((state) => ({
          items: state.items.map((i) =>
            i.id === id ? { ...i, quantity } : i
          ),
        }));
      },

      clear() {
        set({ items: [] });
      },

      count() {
        return get().items.reduce((sum, i) => sum + i.quantity, 0);
      },

      subtotal() {
        return get().items.reduce((sum, i) => sum + i.price * i.quantity, 0);
      },
    }),
    {
      name: "mable_cart",
    }
  )
);
