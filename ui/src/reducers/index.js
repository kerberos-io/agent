import { combineReducers } from 'redux';
import authentication from './authentication';
import agent from './agent';
import wss from './wss';

const rootReducer = combineReducers({
  authentication,
  agent,
  wss,
});

export default rootReducer;
