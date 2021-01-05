import React from 'react';
import PropTypes from 'prop-types';
import './App.css';

function App(props) {
  const { children } = props;
  return (
    <div className="App">
      <header className="App-header">
        { children }
      </header>
      <div className="kerberos-branding">
        Kerberos Open Source
      </div>
    </div>
  );
}

App.propTypes = {
  children: PropTypes.node.isRequired,
};

export default App;
