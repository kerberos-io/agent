import { combineReducers } from 'redux';
import { connectRouter } from 'connected-react-router';
import auth from './auth';

export default history =>
  combineReducers({
    auth,
    router: connectRouter(history),
  });
