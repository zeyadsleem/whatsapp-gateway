import { useEffect, useLayoutEffect, useRef, useState, type ReactElement } from 'react';

export interface ActionMenuItem {
  label: string;
  onClick: () => void;
  danger?: boolean;
}

interface ActionMenuProps {
  label: string;
  items: ActionMenuItem[];
}

interface MenuPosition {
  top: number;
  left: number;
}

const MENU_WIDTH = 208;
const MENU_MARGIN = 8;
const ITEM_HEIGHT = 38;

export function ActionMenu({ label, items }: ActionMenuProps): ReactElement {
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<MenuPosition | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) {
      return;
    }
    const handlePointerDown = (event: PointerEvent): void => {
      const target = event.target;
      if (!(target instanceof Node)) {
        return;
      }
      if (menuRef.current?.contains(target) === true || triggerRef.current?.contains(target) === true) {
        return;
      }
      setOpen(false);
    };
    const handleKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        setOpen(false);
        triggerRef.current?.focus();
      }
    };
    const handleDismiss = (): void => setOpen(false);

    window.addEventListener('pointerdown', handlePointerDown);
    window.addEventListener('keydown', handleKeyDown);
    window.addEventListener('scroll', handleDismiss, true);
    window.addEventListener('resize', handleDismiss);
    return () => {
      window.removeEventListener('pointerdown', handlePointerDown);
      window.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('scroll', handleDismiss, true);
      window.removeEventListener('resize', handleDismiss);
    };
  }, [open]);

  useLayoutEffect(() => {
    if (!open) {
      setPosition(null);
      return;
    }
    const trigger = triggerRef.current;
    if (trigger === null) {
      return;
    }
    const rect = trigger.getBoundingClientRect();
    const menuHeight = items.length * ITEM_HEIGHT + 12;
    const left = Math.max(MENU_MARGIN, Math.min(rect.left, window.innerWidth - MENU_WIDTH - MENU_MARGIN));
    const top =
      rect.bottom + 6 + menuHeight > window.innerHeight - MENU_MARGIN
        ? Math.max(MENU_MARGIN, rect.top - menuHeight - 6)
        : rect.bottom + 6;
    setPosition({ top, left });
  }, [open, items.length]);

  useEffect(() => {
    if (open && position !== null) {
      menuRef.current?.querySelector('button')?.focus();
    }
  }, [open, position]);

  return (
    <div className="action-menu-wrap">
      <button
        ref={triggerRef}
        type="button"
        className="btn btn-secondary btn-sm"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={label}
        onClick={() => setOpen((value) => !value)}
      >
        إجراءات
      </button>
      {open && position !== null ? (
        <div
          ref={menuRef}
          className="action-menu"
          role="menu"
          tabIndex={-1}
          style={{ top: `${position.top}px`, left: `${position.left}px` }}
        >
          {items.map((item) => (
            <button
              key={item.label}
              type="button"
              role="menuitem"
              className={item.danger === true ? 'action-menu-item action-menu-item-danger' : 'action-menu-item'}
              onClick={() => {
                setOpen(false);
                item.onClick();
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
