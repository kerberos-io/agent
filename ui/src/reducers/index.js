import { combineReducers } from 'redux';
import { connectRouter } from 'connected-react-router';
import authentication from './authentication';
import agent from './agent';
import wss from './wss';

export default (history) =>
  combineReducers({
    authentication,
    agent,
    wss,
    router: connectRouter(history),
  });
