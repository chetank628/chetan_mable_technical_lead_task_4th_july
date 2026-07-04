import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { FiSearch, FiStar } from "react-icons/fi";
import { fetchProducts } from "~/lib/api";
import { tracker } from "~/lib/tracker";
import type { Product } from "~/lib/types";
import { useCartStore } from "~/stores/cartStore";
import { useUiStore } from "~/stores/uiStore";
import { EmptyState, ErrorState, Spinner } from "~/components/states";

export function meta() {
  return [{ title: "Mable Shop — Products" }];
}

export default function Home() {
  const [products, setProducts] = useState<Product[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading"
  );
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("all");

  const addItem = useCartStore((s) => s.addItem);
  const pushToast = useUiStore((s) => s.pushToast);

  async function load() {
    setStatus("loading");
    setError("");
    try {
      const list = await fetchProducts(48);
      setProducts(list);
      setStatus("ready");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load products");
      setStatus("error");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const categories = useMemo(() => {
    const set = new Set(products.map((p) => p.category));
    return ["all", ...Array.from(set).sort()];
  }, [products]);

  const filtered = useMemo(() => {
    return products.filter((p) => {
      const matchesQuery = p.title
        .toLowerCase()
        .includes(query.trim().toLowerCase());
      const matchesCat = category === "all" || p.category === category;
      return matchesQuery && matchesCat;
    });
  }, [products, query, category]);

  function handleAdd(product: Product) {
    addItem(product, 1);
    tracker.addToCart({
      id: product.id,
      name: product.title,
      price: product.price,
      category: product.category,
    });
    pushToast(`Added "${product.title}" to cart`, "success");
  }

  if (status === "loading") return <Spinner label="Loading products…" />;
  if (status === "error")
    return <ErrorState message={error} onRetry={load} />;

  return (
    <div>
      <div className="mb-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Products</h1>
          <p className="text-sm text-slate-500">
            {filtered.length} of {products.length} items
          </p>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <label className="relative">
            <span className="sr-only">Search products</span>
            <FiSearch className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search…"
              className="w-full rounded-lg border border-slate-300 py-2 pl-9 pr-3 text-sm focus:border-indigo-500 focus:outline-none sm:w-56"
            />
          </label>
          <label>
            <span className="sr-only">Filter by category</span>
            <select
              value={category}
              onChange={(e) => setCategory(e.target.value)}
              className="w-full rounded-lg border border-slate-300 py-2 pl-3 pr-8 text-sm capitalize focus:border-indigo-500 focus:outline-none sm:w-48"
            >
              {categories.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </label>
        </div>
      </div>

      {filtered.length === 0 ? (
        <EmptyState
          title="No products match your search"
          hint="Try a different search term or category."
        />
      ) : (
        <ul className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-4">
          {filtered.map((product) => (
            <li
              key={product.id}
              className="group flex flex-col overflow-hidden rounded-xl border border-slate-200 bg-white transition-shadow hover:shadow-md"
            >
              <Link
                to={`/product/${product.id}`}
                className="block aspect-square overflow-hidden bg-slate-100"
              >
                <img
                  src={product.thumbnail}
                  alt={product.title}
                  loading="lazy"
                  className="h-full w-full object-cover transition-transform group-hover:scale-105"
                />
              </Link>
              <div className="flex flex-1 flex-col gap-2 p-4">
                <span className="text-xs uppercase tracking-wide text-slate-400">
                  {product.category}
                </span>
                <Link
                  to={`/product/${product.id}`}
                  className="line-clamp-2 font-medium text-slate-800 hover:text-indigo-600"
                >
                  {product.title}
                </Link>
                <div className="mt-auto flex items-center justify-between">
                  <span className="text-lg font-bold text-slate-900">
                    ${product.price.toFixed(2)}
                  </span>
                  <span className="flex items-center gap-1 text-xs text-amber-500">
                    <FiStar aria-hidden /> {product.rating.toFixed(1)}
                  </span>
                </div>
                <button
                  type="button"
                  onClick={() => handleAdd(product)}
                  data-track="AddToCart"
                  data-track-id={product.id}
                  data-track-name={product.title}
                  className="mt-2 rounded-lg bg-indigo-600 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700"
                >
                  Add to cart
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
