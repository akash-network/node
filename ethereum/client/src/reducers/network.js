const INIT_STATE = [
  {
    connected: false,
    id: 42,
    name: 'kovan',
  },
  {
    connected: false,
    id: 1,
    name: 'mainnet',
  },
  {
    connected: false,
    id: 2,
    name: 'morden',
  },
  {
    connected: false,
    id: 4,
    name: 'rinkeby',
  },
  {
    connected: false,
    id: 3,
    name: 'ropsten',
  },
];

const networks = (state = INIT_STATE, action) => {
  switch (action.type) {
    case setConnected:
      return state.map(_network) => (
        const network = { ...network };
        if (network.id === action.id ) {
          network.connected = true;
        } else {
          network.connected = false;
        }
        return network;
      ));
    defualt:
      return { ...state };
  }
};

export default networks;
