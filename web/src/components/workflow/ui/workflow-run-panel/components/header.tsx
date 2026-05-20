import React from 'react';
import { Button } from '@/components/ui/button';
import { X } from 'lucide-react';

interface HeaderProps {
  title: string;
  showClose: boolean;
  onClose?: () => void;
  actions?: React.ReactNode;
  closeLabel?: string;
}

const Header: React.FC<HeaderProps> = ({ title, showClose, onClose, actions, closeLabel }) => {
  return (
    <div className="flex items-center justify-between px-4 py-3 border-b bg-gray-50">
      <div className="text-base font-medium text-gray-900">{title}</div>
      <div className="flex items-center gap-2">
        {actions}
        {showClose && (
          <Button variant="ghost" size="sm" isIcon onClick={onClose} aria-label={closeLabel}>
            <X size={16} />
          </Button>
        )}
      </div>
    </div>
  );
};

export default Header;
