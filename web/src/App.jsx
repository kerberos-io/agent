import React from 'react';
import './App.css';

function App(props) {
  return (
    <div className="App">
      <header className="App-header">
        { props.children }
      </header>
      <div className="kerberos-branding">
        Kerberos Open Source
      </div>
    </div>
  );
}

export default App;
