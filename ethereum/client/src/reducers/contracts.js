import * as actionTypes from '../actions/actionTypes';

const INIT_STATE = {
  master: [
    {
      address: "0x0a3f",
      type: "master",
      version: "0.0.1",
      clientSignature: "0x0123",
      providerSignature: "0x0abc",
    },
  ],
  client: [],
  provider: [],
};

const contracts = (state = INIT_STATE, action) => {
  switch (action.type) {
    case actionTypes.setContracts:
      return { ...state,
        client: action.contracts.
        provider: }
    default:
      return { ...state };
  }
};

export default api;
