import { useEffect, type ReactElement } from 'react';
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from 'react-router-dom';
import { getAdminKey, setUnauthorizedHandler } from './api';
import { DashboardPage } from './pages/DashboardPage';
import { InstancesPage } from './pages/InstancesPage';
import { LoginPage } from './pages/LoginPage';
import { PairPage } from './pages/PairPage';

function RequireAuth({ children }: { children: ReactElement }): ReactElement {
  if (getAdminKey() === null) {
    return <Navigate to="/login" replace />;
  }
  return children;
}

function UnauthorizedRedirect(): null {
  const navigate = useNavigate();

  useEffect(() => {
    setUnauthorizedHandler(() => navigate('/login', { replace: true }));
    return () => setUnauthorizedHandler(null);
  }, [navigate]);

  return null;
}

export function App(): ReactElement {
  return (
    <BrowserRouter basename="/admin">
      <UnauthorizedRedirect />
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          path="/"
          element={
            <RequireAuth>
              <DashboardPage />
            </RequireAuth>
          }
        />
        <Route
          path="/instances"
          element={
            <RequireAuth>
              <InstancesPage />
            </RequireAuth>
          }
        />
        <Route
          path="/instances/:id/pair"
          element={
            <RequireAuth>
              <PairPage />
            </RequireAuth>
          }
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}
