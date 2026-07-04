import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { FiArrowLeft, FiStar } from "react-icons/fi";
import { fetchProduct } from "~/lib/api";
import { tracker } from "~/lib/tracker";
import type { Product } from "~/lib/types";
import { useCartStore } from "~/stores/cartStore";
import { useUiStore } from "~/stores/uiStore";
import { ErrorState, Spinner } from "~/components/states";

export default function ProductDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [product, setProduct] = useState<Product | null>(null);
  const [status, setStatus] = useState<"loading" | "ready" | "error">(
    "loading"
  );
  const [error, setError] = useState("");
  const [activeImage, setActiveImage] = useState(0);

  const addItem = useCartStore((s) => s.addItem);
  const pushToast = useUiStore((s) => s.pushToast);

  async function load() {
    if (!id) return;
    setStatus("loading");
    setError("");
    try {
      const p = await fetchProduct(id);
      setProduct(p);
      setActiveImage(0);
      setStatus("ready");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Product not found");
      setStatus("error");
    }
  }

  useEffect(() => {
    void load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  if (status === "loading") return <Spinner label="Loading product…" />;
  if (status === "error" || !product)
    return <ErrorState message={error || "Product not found"} onRetry={load} />;

  function handleAdd() {
    if (!product) return;
    addItem(product, 1);
    tracker.addToCart({
      id: product.id,
      name: product.title,
      price: product.price,
      category: product.category,
    });
    pushToast(`Added "${product.title}" to cart`, "success");
  }

  const images = product.images?.length ? product.images : [product.thumbnail];

  return (
    <div>
      <Link
        to="/"
        className="mb-6 inline-flex items-center gap-1 text-sm text-slate-500 hover:text-slate-800"
      >
        <FiArrowLeft aria-hidden /> Back to products
      </Link>

      <div className="grid gap-8 md:grid-cols-2">
        <div>
          <div className="aspect-square overflow-hidden rounded-xl border border-slate-200 bg-white">
            <img
              src={images[activeImage]}
              alt={product.title}
              className="h-full w-full object-contain"
            />
          </div>
          {images.length > 1 && (
            <div className="mt-3 flex gap-2 overflow-x-auto">
              {images.map((img, i) => (
                <button
                  key={img}
                  type="button"
                  onClick={() => setActiveImage(i)}
                  aria-label={`View image ${i + 1}`}
                  className={`h-16 w-16 shrink-0 overflow-hidden rounded-lg border ${
                    i === activeImage
                      ? "border-indigo-500"
                      : "border-slate-200"
                  }`}
                >
                  <img
                    src={img}
                    alt=""
                    className="h-full w-full object-cover"
                  />
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="flex flex-col gap-4">
          <span className="text-xs uppercase tracking-wide text-slate-400">
            {product.category} · {product.brand}
          </span>
          <h1 className="text-3xl font-bold text-slate-900">
            {product.title}
          </h1>
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1 text-amber-500">
              <FiStar aria-hidden /> {product.rating.toFixed(1)}
            </span>
            <span className="text-sm text-slate-500">
              {product.stock} in stock
            </span>
          </div>
          <p className="text-slate-600">{product.description}</p>
          <div className="mt-2 text-3xl font-bold text-slate-900">
            ${product.price.toFixed(2)}
          </div>
          <button
            type="button"
            onClick={handleAdd}
            data-track="AddToCart"
            data-track-id={product.id}
            data-track-name={product.title}
            className="mt-2 w-full rounded-lg bg-indigo-600 py-3 font-medium text-white hover:bg-indigo-700 sm:w-auto sm:px-8"
          >
            Add to cart
          </button>
          <button
            type="button"
            onClick={() => {
              handleAdd();
              navigate("/cart");
            }}
            className="w-full rounded-lg border border-slate-300 py-3 font-medium text-slate-700 hover:bg-slate-100 sm:w-auto sm:px-8"
          >
            Buy now
          </button>
        </div>
      </div>
    </div>
  );
}
