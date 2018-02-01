import React, { Component } from 'react';
import PropTypes from 'prop-types';
import { Switch, Route, Redirect } from 'react-router-dom';
import { connect } from 'react-redux';
import { withRouter } from 'react-router';

import { getLoggedIn } from '../actions';

class App extends Component {
  // fetch documents
  componentWillMount = async () => {
    // get if user is logged in
    await this.props.getLoggedIn();
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

      </div>
    );
  }
}

App.propTypes = {
  loggedIn: PropTypes.bool,
  getLoggedIn: PropTypes.func.isRequired,
  role: PropTypes.number,
};

App.defaultProps = {
  loggedIn: false,
  // initalize role at -1. do not load routes until true role id is fetched from the server
  role: -1,
};

const mapStateToProps = state => (
  {
    loggedIn: state.api.loggedIn,
    role: state.api.role,
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
