import logo from './logo.svg';
import './App.css';

function App(props) {
  return (
    <div className="App">
      <header className="App-header">
        { props.children }
      </header>
    </div>
  );
}

export default App;
