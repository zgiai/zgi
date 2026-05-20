import { useState } from 'react';

export type ImageSafeProps = Omit<React.ImgHTMLAttributes<HTMLImageElement>, 'src'> & {
  src: string;
  fallbackComponent?: React.ReactNode;
};

export default function ImageSafe(props: ImageSafeProps) {
  const { alt = '', className, fallbackComponent = null } = props;
  const imgProps = { ...props };
  delete imgProps.fallbackComponent;
  const [isError, setIsError] = useState(false);
  return isError ? (
    fallbackComponent || <div className={`w-full h-full ${className}`}>{alt}</div>
  ) : (
    <img {...imgProps} onError={() => setIsError(true)} />
  );
}
