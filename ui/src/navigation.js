// Lightweight navigation singleton so non-React code (Redux thunks) can
// trigger client-side navigation. The value is set from a component that
// has access to React Router's `useNavigate` hook (see NavigationSetup in
// index.jsx).

let navigatorFn = null;

export const setNavigator = (fn) => {
  navigatorFn = fn;
};

export const navigate = (path, options) => {
  if (navigatorFn) {
    navigatorFn(path, options);
  }
};
