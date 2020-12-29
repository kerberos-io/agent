const auth = (state = {
  username: "",
  role: "",
  loggedIn: false,
  installed: false,
  token: "",
  expire: "",
  loginError: false,
  error: ""
}, action) => {
  switch (action.type) {
    case 'LOGIN':

      // Save token in localStorage
      localStorage.setItem("token", action.token);
      localStorage.setItem("expire", action.expire);
      localStorage.setItem("username", action.username);
      localStorage.setItem("role", action.role);

      return {
        ...state,
        username: action.username,
        role: action.role,
        token: action.token,
        expire: action.expire,
        loggedIn: true,
        installed: true,
        loginError: false,
      };

    case 'LOGIN_FAILED':
      return {
        ...state,
        loginError: true,
        error: action.error
      };

    case 'INSTALLED':
      return {
        ...state,
        installed: true,
        error: ""
      };
      
    case 'LOGOUT':

      // Remove token from localStorage
      localStorage.removeItem("token");
      localStorage.removeItem("expire");
      localStorage.removeItem("username");
      localStorage.removeItem("role");

      return {
        ...state,
        loggedIn: false,
        loginError: false,
        error: ""
      };
    default:
      return state
  }
}

export default auth;
