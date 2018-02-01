const INIT_STATE = {
  "0.0.1": {
    client: [],
    provide: [],
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
