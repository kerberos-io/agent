import {
  WEBSOCKET_BROKEN,
  WEBSOCKET_CLOSED,
  WEBSOCKET_CONNECT,
  WEBSOCKET_DISCONNECT,
  WEBSOCKET_MESSAGE,
  WEBSOCKET_OPEN,
  WEBSOCKET_SEND,
} from '@giantmachines/redux-websocket';

export const WEBSOCKET_PREFIX = 'REDUX_WEBSOCKET';
export const REDUX_WEBSOCKET_BROKEN = `${WEBSOCKET_PREFIX}::${WEBSOCKET_BROKEN}`;
export const REDUX_WEBSOCKET_OPEN = `${WEBSOCKET_PREFIX}::${WEBSOCKET_OPEN}`;
export const REDUX_WEBSOCKET_CLOSED = `${WEBSOCKET_PREFIX}::${WEBSOCKET_CLOSED}`;
export const REDUX_WEBSOCKET_MESSAGE = `${WEBSOCKET_PREFIX}::${WEBSOCKET_MESSAGE}`;
export const REDUX_WEBSOCKET_CONNECT = `${WEBSOCKET_PREFIX}::${WEBSOCKET_CONNECT}`;
export const REDUX_WEBSOCKET_DISCONNECT = `${WEBSOCKET_PREFIX}::${WEBSOCKET_DISCONNECT}`;
export const REDUX_WEBSOCKET_SEND = `${WEBSOCKET_PREFIX}::${WEBSOCKET_SEND}`;

const wss = (
  state = {
    connected: false,
    messages: {},
    events: [],
    url: null,
  },
  action
) => {
  switch (action.type) {
    case 'INTERNAL::CLEAR_MESSAGE_LOG':
      return {
        ...state,
        messages: [],
      };

    case REDUX_WEBSOCKET_CONNECT:
      return {
        ...state,
        url: action.payload.url,
      };

    case REDUX_WEBSOCKET_OPEN:
      return {
        ...state,
        connected: true,
      };

    case REDUX_WEBSOCKET_BROKEN:
    case REDUX_WEBSOCKET_CLOSED:
      return {
        ...state,
        connected: false,
      };

    case REDUX_WEBSOCKET_MESSAGE:
      const { payload } = action;
      const message = JSON.parse(payload.message);
      if (message) {
        const { key, value } = message;
        switch (key) {
          case 'pod-events':
            return {
              ...state,
              events: [...state.events, value],
            };
          case 'watching-pods': // We do not need to do anything
            return state;

          default: // Anything else should be logging (to be refactored).
            if (!state.messages[key]) {
              return {
                ...state,
                messages: {
                  ...state.messages,
                  [key]: [
                    {
                      data: value,
                      origin: action.payload.origin,
                      timestamp: action.meta.timestamp,
                      type: 'INCOMING',
                    },
                  ],
                },
              };
            }
            return {
              ...state,
              messages: {
                ...state.messages,
                [key]: [
                  ...state.messages[key],
                  {
                    data: value,
                    origin: action.payload.origin,
                    timestamp: action.meta.timestamp,
                    type: 'INCOMING',
                  },
                ],
              },
            };
        }
      }
      break;
    default:
      return state;
  }
  return state;
};

export default wss;
