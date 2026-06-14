'use client';

import FileManagementContent from '@/components/files/file-management-content';

const FileManagementPage = () => {
  return (
    <div className="flex flex-col h-full overflow-y-auto">
      <FileManagementContent enableAIChatContext />
    </div>
  );
};

export default FileManagementPage;
