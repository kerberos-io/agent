import { combineReducers } from 'redux';
import { connectRouter } from 'connected-react-router';
import authentication from './authentication';
import agent from './agent';

export default (history) =>
  combineReducers({
    authentication,
    agent,
    router: connectRouter(history),
  });
