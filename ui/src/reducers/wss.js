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
    images: [],
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
      const m = JSON.parse(payload.message);
      if (m) {
        const { message_type, message } = m;
        switch (message_type) {
          case 'image':
            const { base64 } = message;
            return {
              ...state,
              images: [base64],
            };

          default: // Anything else should be logging (to be refactored).
        }
      }
      break;
    default:
      return state;
  }
  return state;
};

export default wss;
