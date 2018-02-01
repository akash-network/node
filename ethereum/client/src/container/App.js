import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Switch, Route, Redirect } from 'react-router-dom';
import { connect } from 'react-redux';
import { withRouter } from 'react-router';

import WarningModal from '../component'
import { initWeb3 } from '../actions';

class App extends Component {
  componentWillMount = async () => {
    // init web3
    await this.props.initWeb3();
  };

  render() {
    return (
      <div className="App">
          <Nav />
          <Switch>
            <Route exact path="/" component={Dashboard} />
            <Route render={() => {
              return (<h2 className="text-center">Not Found</h2>);
            }}
            />
          </Switch>
          { // show warning modal if the client is not connected to a supported network
            !this.props.connected || !this.props.supportedNetwork <WarningModal />
          }
      </div>
    );
  }
}

App.propTypes = {
  connected: PropTypes.bool,
  supportedNetwork: PropTypes.func.isRequired,
};

App.defaultProps = {
  connected: false,
  supportedNetwork: false,
};

const mapStateToProps = state => (
  {
    loggedIn: state.api.loggedIn,
  }
);

const mapDispatchToProps = dispatch => (
  {
    getLoggedIn: () => (
      dispatch(getLoggedIn())
    ),
  }
);


export default withRouter(connect(
  mapStateToProps,
  mapDispatchToProps,
)(App));
