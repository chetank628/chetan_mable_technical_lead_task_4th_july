import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { FiCheck } from "react-icons/fi";
import { tracker } from "~/lib/tracker";
import { useCartStore } from "~/stores/cartStore";
import { useUiStore } from "~/stores/uiStore";
import { useRequireAuth } from "~/hooks/useRequireAuth";
import { EmptyState, Spinner } from "~/components/states";

type Step = "shipping" | "payment" | "review" | "done";

const STEPS: { key: Step; label: string }[] = [
  { key: "shipping", label: "Shipping" },
  { key: "payment", label: "Payment" },
  { key: "review", label: "Review" },
];

interface Shipping {
  name: string;
  address: string;
  city: string;
  postcode: string;
  country: string;
}

interface Payment {
  card: string;
  expiry: string;
  cvc: string;
}

export default function Checkout() {
  const navigate = useNavigate();
  const { ready } = useRequireAuth();
  const items = useCartStore((s) => s.items);
  const subtotal = useCartStore((s) => s.subtotal);
  const clear = useCartStore((s) => s.clear);
  const pushToast = useUiStore((s) => s.pushToast);

  const [step, setStep] = useState<Step>("shipping");
  const [placing, setPlacing] = useState(false);
  const [shipping, setShipping] = useState<Shipping>({
    name: "",
    address: "",
    city: "",
    postcode: "",
    country: "",
  });
  const [payment, setPayment] = useState<Payment>({
    card: "",
    expiry: "",
    cvc: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});

  const total = subtotal();

  // Emit a Checkout event once when the flow opens with a non-empty cart.
  useEffect(() => {
    if (ready && items.length > 0) {
      tracker.checkout({
        items: items.map((i) => ({ id: i.id, qty: i.quantity })),
        item_count: items.reduce((n, i) => n + i.quantity, 0),
        amount: total,
        currency: "USD",
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready]);

  if (!ready) return <Spinner label="Checking your session…" />;

  // Empty-cart checkout is blocked.
  if (items.length === 0 && step !== "done") {
    return (
      <EmptyState
        title="Nothing to check out"
        hint="Your cart is empty."
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

  function validateShipping() {
    const e: Record<string, string> = {};
    if (!shipping.name.trim()) e.name = "Name is required";
    if (!shipping.address.trim()) e.address = "Address is required";
    if (!shipping.city.trim()) e.city = "City is required";
    if (!shipping.postcode.trim()) e.postcode = "Postcode is required";
    if (!shipping.country.trim()) e.country = "Country is required";
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  function validatePayment() {
    const e: Record<string, string> = {};
    if (!/^\d{12,19}$/.test(payment.card.replace(/\s/g, "")))
      e.card = "Enter a valid card number";
    if (!/^\d{2}\/\d{2}$/.test(payment.expiry))
      e.expiry = "Use MM/YY format";
    if (!/^\d{3,4}$/.test(payment.cvc)) e.cvc = "Invalid CVC";
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  function goToPayment() {
    if (validateShipping()) {
      setErrors({});
      setStep("payment");
    }
  }

  function goToReview() {
    if (validatePayment()) {
      setErrors({});
      // Never send raw card data — only a non-sensitive signal.
      tracker.paymentInfoAdded({
        method: "card",
        last4: payment.card.replace(/\s/g, "").slice(-4),
        amount: total,
        currency: "USD",
      });
      setStep("review");
    }
  }

  async function placeOrder() {
    setPlacing(true);
    const orderId = `ORD-${Date.now()}`;
    // Simulate order processing latency.
    await new Promise((r) => setTimeout(r, 700));
    tracker.purchase({
      order_id: orderId,
      amount: total,
      currency: "USD",
      items: items.map((i) => ({ id: i.id, qty: i.quantity, price: i.price })),
      item_count: items.reduce((n, i) => n + i.quantity, 0),
    });
    clear();
    setPlacing(false);
    setStep("done");
    pushToast("Order placed! Thank you.", "success");
    // Redirect back to the shop after a short success screen.
    setTimeout(() => navigate("/"), 2500);
  }

  if (step === "done") {
    return (
      <div className="flex flex-col items-center justify-center gap-4 py-24 text-center">
        <div className="flex h-16 w-16 items-center justify-center rounded-full bg-emerald-100 text-emerald-600">
          <FiCheck className="h-8 w-8" aria-hidden />
        </div>
        <h1 className="text-2xl font-bold text-slate-900">Order confirmed</h1>
        <p className="text-slate-600">
          Thanks for your purchase. Redirecting you back to the shop…
        </p>
        <Link to="/" className="text-indigo-600 hover:underline">
          Continue shopping now
        </Link>
      </div>
    );
  }

  const activeIndex = STEPS.findIndex((s) => s.key === step);

  return (
    <div className="mx-auto max-w-3xl">
      <h1 className="mb-6 text-2xl font-bold text-slate-900">Checkout</h1>

      {/* Step indicator */}
      <ol className="mb-8 flex items-center gap-2">
        {STEPS.map((s, i) => (
          <li key={s.key} className="flex flex-1 items-center gap-2">
            <span
              className={`flex h-8 w-8 items-center justify-center rounded-full text-sm font-semibold ${
                i <= activeIndex
                  ? "bg-indigo-600 text-white"
                  : "bg-slate-200 text-slate-500"
              }`}
            >
              {i < activeIndex ? <FiCheck aria-hidden /> : i + 1}
            </span>
            <span
              className={`text-sm font-medium ${
                i <= activeIndex ? "text-slate-900" : "text-slate-400"
              }`}
            >
              {s.label}
            </span>
            {i < STEPS.length - 1 && (
              <span className="mx-2 h-px flex-1 bg-slate-200" />
            )}
          </li>
        ))}
      </ol>

      <div className="rounded-xl border border-slate-200 bg-white p-6">
        {step === "shipping" && (
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              goToPayment();
            }}
          >
            <Field
              label="Full name"
              value={shipping.name}
              error={errors.name}
              onChange={(v) => setShipping({ ...shipping, name: v })}
            />
            <Field
              label="Address"
              value={shipping.address}
              error={errors.address}
              onChange={(v) => setShipping({ ...shipping, address: v })}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                label="City"
                value={shipping.city}
                error={errors.city}
                onChange={(v) => setShipping({ ...shipping, city: v })}
              />
              <Field
                label="Postcode"
                value={shipping.postcode}
                error={errors.postcode}
                onChange={(v) => setShipping({ ...shipping, postcode: v })}
              />
            </div>
            <Field
              label="Country"
              value={shipping.country}
              error={errors.country}
              onChange={(v) => setShipping({ ...shipping, country: v })}
            />
            <StepButtons primaryLabel="Continue to payment" />
          </form>
        )}

        {step === "payment" && (
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              goToReview();
            }}
          >
            <Field
              label="Card number"
              value={payment.card}
              error={errors.card}
              placeholder="4242 4242 4242 4242"
              onChange={(v) => setPayment({ ...payment, card: v })}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <Field
                label="Expiry (MM/YY)"
                value={payment.expiry}
                error={errors.expiry}
                placeholder="12/29"
                onChange={(v) => setPayment({ ...payment, expiry: v })}
              />
              <Field
                label="CVC"
                value={payment.cvc}
                error={errors.cvc}
                placeholder="123"
                onChange={(v) => setPayment({ ...payment, cvc: v })}
              />
            </div>
            <p className="text-xs text-slate-400">
              Demo only — do not enter real card details. Card data never leaves
              the browser; only a non-sensitive signal is tracked.
            </p>
            <StepButtons
              primaryLabel="Review order"
              onBack={() => setStep("shipping")}
            />
          </form>
        )}

        {step === "review" && (
          <div className="flex flex-col gap-5">
            <section>
              <h2 className="mb-2 font-semibold text-slate-800">Items</h2>
              <ul className="flex flex-col gap-2 text-sm">
                {items.map((i) => (
                  <li key={i.id} className="flex justify-between">
                    <span className="text-slate-600">
                      {i.title} × {i.quantity}
                    </span>
                    <span className="font-medium">
                      ${(i.price * i.quantity).toFixed(2)}
                    </span>
                  </li>
                ))}
              </ul>
              <div className="mt-3 flex justify-between border-t border-slate-200 pt-3 font-semibold">
                <span>Total</span>
                <span>${total.toFixed(2)}</span>
              </div>
            </section>

            <section className="grid gap-4 text-sm sm:grid-cols-2">
              <div>
                <h3 className="font-semibold text-slate-800">Ship to</h3>
                <p className="text-slate-600">
                  {shipping.name}
                  <br />
                  {shipping.address}
                  <br />
                  {shipping.city}, {shipping.postcode}
                  <br />
                  {shipping.country}
                </p>
              </div>
              <div>
                <h3 className="font-semibold text-slate-800">Payment</h3>
                <p className="text-slate-600">
                  Card ending {payment.card.replace(/\s/g, "").slice(-4)}
                </p>
              </div>
            </section>

            <div className="flex justify-between">
              <button
                type="button"
                onClick={() => setStep("payment")}
                className="rounded-lg border border-slate-300 px-5 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
              >
                Back
              </button>
              <button
                type="button"
                onClick={placeOrder}
                disabled={placing}
                className="rounded-lg bg-indigo-600 px-6 py-2.5 text-sm font-medium text-white hover:bg-indigo-700 disabled:opacity-60"
              >
                {placing ? "Placing order…" : `Place order · $${total.toFixed(2)}`}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function Field({
  label,
  value,
  onChange,
  error,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  error?: string;
  placeholder?: string;
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-sm font-medium text-slate-700">{label}</span>
      <input
        type="text"
        value={value}
        placeholder={placeholder}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={!!error}
        className={`rounded-lg border px-3 py-2 text-sm focus:outline-none ${
          error
            ? "border-rose-400 focus:border-rose-500"
            : "border-slate-300 focus:border-indigo-500"
        }`}
      />
      {error && <span className="text-xs text-rose-600">{error}</span>}
    </label>
  );
}

function StepButtons({
  primaryLabel,
  onBack,
}: {
  primaryLabel: string;
  onBack?: () => void;
}) {
  return (
    <div className="flex justify-between">
      {onBack ? (
        <button
          type="button"
          onClick={onBack}
          className="rounded-lg border border-slate-300 px-5 py-2.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
        >
          Back
        </button>
      ) : (
        <span />
      )}
      <button
        type="submit"
        className="rounded-lg bg-indigo-600 px-6 py-2.5 text-sm font-medium text-white hover:bg-indigo-700"
      >
        {primaryLabel}
      </button>
    </div>
  );
}
