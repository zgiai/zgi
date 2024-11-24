import React from 'react';
import { MessageSquare, ChevronDown, Github } from 'lucide-react';

const Header = () => {
  return (
    <header className="bg-white border-b border-gray-200 p-4 flex justify-between items-center">
      <div className="flex items-center">
        <MessageSquare className="text-blue-500 mr-2" size={24} />
        <span className="font-semibold">ZGIChat</span>
        <ChevronDown size={18} className="ml-1" />
      </div>
      <div className="flex items-center">
        {/* <Star className="mr-4" size={18} /> */}
        <Github />
        {/* <span className="text-sm">Star on Github</span> */}
        <button className="ml-4 px-3 py-1 bg-gray-200 rounded-md text-sm">Feedback</button>
      </div>
    </header>
  );
};

export default Header;