import { combineReducers } from 'redux';

import abi from './abi';
import contracts from './contracts';
import network from './network';


const reducer = combineReducers({
  abi,
  contracts,
  network,
});

export default reducer;
