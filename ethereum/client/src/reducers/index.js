import { combineReducers } from 'redux';

import abi from './abi';
import contracts from './contracts';
import networks from './networks';

const reducer = combineReducers({
  abi,
  contracts,
  networks,
});

export default reducer;
