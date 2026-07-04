import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { useAuthStore } from "~/stores/authStore";

export function meta() {
  return [{ title: "Sign in — Mable Shop" }];
}

export default function Login() {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const login = useAuthStore((s) => s.login);
  const status = useAuthStore((s) => s.status);
  const restore = useAuthStore((s) => s.restore);

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errors, setErrors] = useState<{ form?: string; email?: string; password?: string }>(
    {}
  );
  const [submitting, setSubmitting] = useState(false);

  const next = params.get("next") || "/";

  // If a session already exists, skip the login form.
  useEffect(() => {
    if (status === "idle") void restore();
    if (status === "authenticated") navigate(next, { replace: true });
  }, [status, restore, navigate, next]);

  function validate() {
    const e: typeof errors = {};
    if (!email.includes("@")) e.email = "Enter a valid email address";
    if (password.length < 8) e.password = "Password must be at least 8 characters";
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  async function onSubmit(ev: React.FormEvent) {
    ev.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    setErrors({});
    try {
      await login(email.trim(), password);
      navigate(next, { replace: true });
    } catch (e) {
      setErrors({ form: e instanceof Error ? e.message : "Login failed" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell title="Welcome back" subtitle="Sign in to your Mable account">
      <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate>
        {errors.form && (
          <p role="alert" className="rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {errors.form}
          </p>
        )}
        <label className="flex flex-col gap-1">
          <span className="text-sm font-medium text-slate-700">Email</span>
          <input
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className={inputClass(!!errors.email)}
          />
          {errors.email && <span className="text-xs text-rose-600">{errors.email}</span>}
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-sm font-medium text-slate-700">Password</span>
          <input
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className={inputClass(!!errors.password)}
          />
          {errors.password && (
            <span className="text-xs text-rose-600">{errors.password}</span>
          )}
        </label>
        <button
          type="submit"
          disabled={submitting}
          className="mt-2 rounded-lg bg-indigo-600 py-2.5 font-medium text-white hover:bg-indigo-700 disabled:opacity-60"
        >
          {submitting ? "Signing in…" : "Sign in"}
        </button>
      </form>
      <p className="mt-6 text-center text-sm text-slate-500">
        No account?{" "}
        <Link to="/signup" className="font-medium text-indigo-600 hover:underline">
          Create one
        </Link>
      </p>
    </AuthShell>
  );
}

export function inputClass(hasError: boolean) {
  return `rounded-lg border px-3 py-2 text-sm focus:outline-none ${
    hasError
      ? "border-rose-400 focus:border-rose-500"
      : "border-slate-300 focus:border-indigo-500"
  }`;
}

export function AuthShell({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
}) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-slate-50 px-4">
      <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-8 shadow-sm">
        <Link to="/" className="mb-6 flex justify-center">
          <span className="rounded-md bg-indigo-600 px-3 py-1.5 text-lg font-bold text-white">
            Mable
          </span>
        </Link>
        <h1 className="text-center text-xl font-bold text-slate-900">{title}</h1>
        <p className="mb-6 text-center text-sm text-slate-500">{subtitle}</p>
        {children}
      </div>
    </main>
  );
}
