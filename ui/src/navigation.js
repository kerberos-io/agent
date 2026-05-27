// Lightweight navigation singleton so non-React code (Redux thunks) can
// trigger client-side navigation. Backed by the shared `history` instance
// passed to react-router's <Router>.
import history from './history';

export const setNavigator = () => {
  // Kept for API compatibility; no-op since history is module-scoped.
};

export const navigate = (path) => {
  history.push(path);
};
