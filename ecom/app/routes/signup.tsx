import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useAuthStore } from "~/stores/authStore";
import { AuthShell, inputClass } from "./login";

export function meta() {
  return [{ title: "Create account — Mable Shop" }];
}

export default function Signup() {
  const navigate = useNavigate();
  const signup = useAuthStore((s) => s.signup);
  const status = useAuthStore((s) => s.status);
  const restore = useAuthStore((s) => s.restore);

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [errors, setErrors] = useState<{
    form?: string;
    email?: string;
    password?: string;
    confirm?: string;
  }>({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (status === "idle") void restore();
    if (status === "authenticated") navigate("/", { replace: true });
  }, [status, restore, navigate]);

  function validate() {
    const e: typeof errors = {};
    if (!email.includes("@")) e.email = "Enter a valid email address";
    if (password.length < 8)
      e.password = "Password must be at least 8 characters";
    if (password !== confirm) e.confirm = "Passwords do not match";
    setErrors(e);
    return Object.keys(e).length === 0;
  }

  async function onSubmit(ev: React.FormEvent) {
    ev.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    setErrors({});
    try {
      await signup(email.trim(), password);
      navigate("/", { replace: true });
    } catch (e) {
      setErrors({ form: e instanceof Error ? e.message : "Signup failed" });
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell title="Create your account" subtitle="Start shopping in seconds">
      <form className="flex flex-col gap-4" onSubmit={onSubmit} noValidate>
        {errors.form && (
          <p
            role="alert"
            className="rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-700"
          >
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
          {errors.email && (
            <span className="text-xs text-rose-600">{errors.email}</span>
          )}
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-sm font-medium text-slate-700">Password</span>
          <input
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className={inputClass(!!errors.password)}
          />
          {errors.password && (
            <span className="text-xs text-rose-600">{errors.password}</span>
          )}
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-sm font-medium text-slate-700">
            Confirm password
          </span>
          <input
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className={inputClass(!!errors.confirm)}
          />
          {errors.confirm && (
            <span className="text-xs text-rose-600">{errors.confirm}</span>
          )}
        </label>
        <button
          type="submit"
          disabled={submitting}
          className="mt-2 rounded-lg bg-indigo-600 py-2.5 font-medium text-white hover:bg-indigo-700 disabled:opacity-60"
        >
          {submitting ? "Creating account…" : "Create account"}
        </button>
      </form>
      <p className="mt-6 text-center text-sm text-slate-500">
        Already have an account?{" "}
        <Link to="/login" className="font-medium text-indigo-600 hover:underline">
          Sign in
        </Link>
      </p>
    </AuthShell>
  );
}
