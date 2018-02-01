import * as actionTypes from '../actions/actionTypes';

const INIT_STATE = {
  master: [
    {
      address: "0x0",
      type: "master",
      version: "0.0.1",
    },
  ],
  client: [],
  provider: [],
};

const contracts = (state = INIT_STATE, action) => {
  switch (action.type) {
    case actionTypes.setContracts:
      return { ...state, action.contract }
    default:
      return { ...state };
  }
};

export default api;
