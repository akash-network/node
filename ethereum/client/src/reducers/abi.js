const INIT_STATE = {
  "0.0.1": {
    client: [],
    provider: [],
    master: [],
  },
};

const abi = (state = INIT_STATE, action) => {
  switch (action.type) {
    default:
      return { ..state };
  }
};

export default abi;
