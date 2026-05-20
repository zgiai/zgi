import React from 'react';

interface ConversationWarperProps {
  children?: React.ReactNode;
  className?: string;
}

// Placeholder for group modes; returns wrapper only
const ConversationWarper: React.FC<ConversationWarperProps> = ({ children, className }) => {
  return <div className={className}>{children}</div>;
};

export default ConversationWarper;
