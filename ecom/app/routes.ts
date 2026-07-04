import {
  type RouteConfig,
  index,
  layout,
  route,
} from "@react-router/dev/routes";

export default [
  // Auth pages render outside the main shell (no nav chrome).
  route("login", "routes/login.tsx"),
  route("signup", "routes/signup.tsx"),

  // Everything else shares the header / consent banner / toaster shell.
  layout("routes/_shell.tsx", [
    index("routes/home.tsx"),
    route("product/:id", "routes/product.tsx"),
    route("cart", "routes/cart.tsx"),
    route("checkout", "routes/checkout.tsx"),
    route("dashboard", "routes/dashboard.tsx"),
  ]),
] satisfies RouteConfig;
