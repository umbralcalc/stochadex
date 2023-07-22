import React from 'react';
import Dashboard from './Dashboard';

const App: React.FC = () => {
  return (
    <div>
      <Dashboard />
      <h5>Zoom: Mousewheel or Shift + Click + Drag</h5>
      <h5>Pan: Ctrl + Click + Drag</h5>
      <h5>Reset: r Key</h5>
    </div>
  );
};

export default App;
