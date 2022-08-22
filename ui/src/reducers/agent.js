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
        config: action.config,
      };

    case 'UPDATE_CONFIG':
      if (action.field === '') {
        return {
          ...state,
          config: {
            ...state.config,
            config: action.value,
          },
        };
      }

      const levels = action.field.split('.');
      if (levels.length === 1) {
        return {
          ...state,
          config: {
            ...state.config,
            config: {
              ...state.config.config,
              [action.field]: action.value,
            },
          },
        };
      }

      return {
        ...state,
        config: {
          ...state.config,
          config: {
            ...state.config.config,
            [levels[0]]: {
              ...state.config.config[levels[0]],
              [levels[1]]: action.value,
            },
          },
        },
      };

    default:
      return state;
  }
};

export default agent;
