const agent = (
  state = {
    config: {},
  },
  action
) => {
  switch (action.type) {
    case 'GET_CONFIG':
      return {
        ...state,
        config: action.payload,
      };

    default:
      return state;
  }
};

export default agent;
