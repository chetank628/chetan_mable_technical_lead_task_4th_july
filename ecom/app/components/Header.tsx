import { Link, NavLink, useNavigate } from "react-router";
import { FiBarChart2, FiLogOut, FiShoppingCart, FiUser } from "react-icons/fi";
import { useAuthStore } from "~/stores/authStore";
import { useCartStore } from "~/stores/cartStore";
import { useUiStore } from "~/stores/uiStore";

export function Header() {
  const navigate = useNavigate();
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const items = useCartStore((s) => s.items);
  const pushToast = useUiStore((s) => s.pushToast);

  const cartCount = items.reduce((sum, i) => sum + i.quantity, 0);

  async function handleLogout() {
    await logout();
    pushToast("Signed out", "info");
    navigate("/login");
  }

  const navClass = ({ isActive }: { isActive: boolean }) =>
    `text-sm font-medium transition-colors ${
      isActive ? "text-indigo-600" : "text-slate-600 hover:text-slate-900"
    }`;

  return (
    <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/90 backdrop-blur">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-4">
        <Link to="/" className="flex items-center gap-2 text-lg font-bold">
          <span className="rounded-md bg-indigo-600 px-2 py-1 text-white">
            Mable
          </span>
          <span className="hidden text-slate-800 sm:inline">Shop</span>
        </Link>

        <nav aria-label="Primary" className="flex items-center gap-5">
          <NavLink to="/" className={navClass} end>
            Products
          </NavLink>
          {user && (
            <NavLink to="/dashboard" className={navClass}>
              <span className="flex items-center gap-1">
                <FiBarChart2 aria-hidden /> Stats
              </span>
            </NavLink>
          )}
        </nav>

        <div className="flex items-center gap-4">
          <Link
            to="/cart"
            className="relative text-slate-600 hover:text-slate-900"
            aria-label={`Cart with ${cartCount} items`}
          >
            <FiShoppingCart className="h-6 w-6" aria-hidden />
            {cartCount > 0 && (
              <span className="absolute -right-2 -top-2 flex h-5 min-w-5 items-center justify-center rounded-full bg-indigo-600 px-1 text-xs font-semibold text-white">
                {cartCount}
              </span>
            )}
          </Link>

          {user ? (
            <div className="flex items-center gap-3">
              <span className="hidden items-center gap-1 text-sm text-slate-600 sm:flex">
                <FiUser aria-hidden /> {user.email}
              </span>
              <button
                type="button"
                onClick={handleLogout}
                className="flex items-center gap-1 rounded-lg border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100"
              >
                <FiLogOut aria-hidden /> Logout
              </button>
            </div>
          ) : (
            <Link
              to="/login"
              className="rounded-lg bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-indigo-700"
            >
              Sign in
            </Link>
          )}
        </div>
      </div>
    </header>
  );
}
