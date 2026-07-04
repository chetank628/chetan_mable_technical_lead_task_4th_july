import { Link, useNavigate } from "react-router";
import { FiMinus, FiPlus, FiTrash2 } from "react-icons/fi";
import { useCartStore } from "~/stores/cartStore";
import { EmptyState } from "~/components/states";

export default function Cart() {
  const navigate = useNavigate();
  const items = useCartStore((s) => s.items);
  const setQuantity = useCartStore((s) => s.setQuantity);
  const removeItem = useCartStore((s) => s.removeItem);
  const subtotal = useCartStore((s) => s.subtotal);

  if (items.length === 0) {
    return (
      <EmptyState
        title="Your cart is empty"
        hint="Browse the catalog and add a few products."
        action={
          <Link
            to="/"
            className="rounded-lg bg-indigo-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-indigo-700"
          >
            Browse products
          </Link>
        }
      />
    );
  }

  const total = subtotal();

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold text-slate-900">Your cart</h1>
      <div className="grid gap-8 lg:grid-cols-[1fr_320px]">
        <ul className="flex flex-col gap-3">
          {items.map((item) => (
            <li
              key={item.id}
              className="flex items-center gap-4 rounded-xl border border-slate-200 bg-white p-3"
            >
              <img
                src={item.thumbnail}
                alt={item.title}
                className="h-20 w-20 rounded-lg object-cover"
              />
              <div className="flex-1">
                <Link
                  to={`/product/${item.id}`}
                  className="font-medium text-slate-800 hover:text-indigo-600"
                >
                  {item.title}
                </Link>
                <p className="text-sm text-slate-500">
                  ${item.price.toFixed(2)} each
                </p>
              </div>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  aria-label="Decrease quantity"
                  onClick={() => setQuantity(item.id, item.quantity - 1)}
                  className="rounded-md border border-slate-300 p-1.5 hover:bg-slate-100"
                >
                  <FiMinus className="h-4 w-4" aria-hidden />
                </button>
                <span className="w-8 text-center font-medium" aria-live="polite">
                  {item.quantity}
                </span>
                <button
                  type="button"
                  aria-label="Increase quantity"
                  onClick={() => setQuantity(item.id, item.quantity + 1)}
                  className="rounded-md border border-slate-300 p-1.5 hover:bg-slate-100"
                >
                  <FiPlus className="h-4 w-4" aria-hidden />
                </button>
              </div>
              <div className="w-20 text-right font-semibold text-slate-900">
                ${(item.price * item.quantity).toFixed(2)}
              </div>
              <button
                type="button"
                aria-label={`Remove ${item.title}`}
                onClick={() => removeItem(item.id)}
                className="rounded-md p-2 text-slate-400 hover:bg-rose-50 hover:text-rose-600"
              >
                <FiTrash2 className="h-5 w-5" aria-hidden />
              </button>
            </li>
          ))}
        </ul>

        <aside className="h-fit rounded-xl border border-slate-200 bg-white p-5">
          <h2 className="mb-4 text-lg font-semibold">Summary</h2>
          <dl className="flex flex-col gap-2 text-sm">
            <div className="flex justify-between">
              <dt className="text-slate-500">Subtotal</dt>
              <dd className="font-medium">${total.toFixed(2)}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-slate-500">Shipping</dt>
              <dd className="font-medium">Free</dd>
            </div>
            <div className="mt-2 flex justify-between border-t border-slate-200 pt-3 text-base">
              <dt className="font-semibold">Total</dt>
              <dd className="font-bold">${total.toFixed(2)}</dd>
            </div>
          </dl>
          <button
            type="button"
            onClick={() => navigate("/checkout")}
            className="mt-5 w-full rounded-lg bg-indigo-600 py-3 font-medium text-white hover:bg-indigo-700"
          >
            Proceed to checkout
          </button>
        </aside>
      </div>
    </div>
  );
}
