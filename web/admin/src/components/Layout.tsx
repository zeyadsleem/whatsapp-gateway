import type { ReactElement, ReactNode } from 'react';
import { NavLink, useNavigate } from 'react-router-dom';
import { clearAdminKey } from '../api';

interface LayoutProps {
  title: string;
  children: ReactNode;
}

function navLinkClass({ isActive }: { isActive: boolean }): string {
  return isActive ? 'nav-link nav-link-active' : 'nav-link';
}

export function Layout({ title, children }: LayoutProps): ReactElement {
  const navigate = useNavigate();

  const handleLogout = (): void => {
    clearAdminKey();
    void navigate('/login', { replace: true });
  };

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
            </svg>
          </span>
          <span className="brand-text">بوابة واتساب</span>
        </div>
        <nav className="sidebar-nav" aria-label="التنقل الرئيسي">
          <NavLink to="/" end className={navLinkClass}>
            لوحة التحكم
          </NavLink>
          <NavLink to="/instances" className={navLinkClass}>
            الاتصالات
          </NavLink>
        </nav>
        <button type="button" className="btn btn-ghost logout-btn" onClick={handleLogout}>
          تسجيل الخروج
        </button>
      </aside>
      <div className="main-area">
        <header className="topbar">
          <h1>{title}</h1>
        </header>
        <main className="content">{children}</main>
      </div>
    </div>
  );
}
