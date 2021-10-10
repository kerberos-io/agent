import * as React from 'react';
import { render, screen, test, expect } from '@testing-library/react';
import App from './App';

/* Test default values by rendering a context consumer without a
 * matching provider
 */
test('App.jsx verify if children are rendered.', () => {
  render(
    <App>
      <div>Kerberos Open Source</div>
    </App>
  );
  expect(screen.getByText(/^Kerberos Open Source/)).toHaveTextContent(
    'Kerberos Open Source'
  );
});
