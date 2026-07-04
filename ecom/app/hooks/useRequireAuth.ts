import { useEffect } from "react";
import { useNavigate } from "react-router";
import { useAuthStore } from "~/stores/authStore";

// Redirects to /login once the session is known to be unauthenticated. While
// the session is still being restored ("idle"/"loading") it returns ready=false
// so the caller can show a spinner instead of flashing the login redirect.
export function useRequireAuth(): { ready: boolean } {
  const navigate = useNavigate();
  const status = useAuthStore((s) => s.status);

  useEffect(() => {
    if (status === "unauthenticated") {
      const next = encodeURIComponent(
        window.location.pathname + window.location.search
      );
      navigate(`/login?next=${next}`, { replace: true });
    }
  }, [status, navigate]);

  return { ready: status === "authenticated" };
}
