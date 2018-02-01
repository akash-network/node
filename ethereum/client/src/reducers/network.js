const INIT_STATE = {
  kovan: {
    connected: false,
    id: 42,
  },
  mainnet: {
    connected: false,
    id: 1,
  },
  morden: {
    connected: false,
    id: 2,
  },
  rinkeby: {
    connected: false,
    id: 4,
  },
  ropsten: {
    connected: false,
    id: 3,
  },
};

const network = (state = INIT_STATE, action) => {
  switch (action.type) {
    case setConnected:
      // set connection true where netid === id
      // set all others false
      return;
    defualt:
      return;
  }
};

export default network;
