import Link from 'next/link';
import {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import type { DatasetFolder } from '@/services/types/dataset-folder';

export interface DatasetBreadcrumbsProps {
  ancestors: DatasetFolder[];
  titleText: string;
}

/**
 * Breadcrumbs for subfolder navigation.
 */
export function DatasetBreadcrumbs({ ancestors, titleText }: DatasetBreadcrumbsProps) {
  if (!ancestors || ancestors.length === 0) return null;

  return (
    <Breadcrumb>
      <BreadcrumbList className="text-lg">
        <BreadcrumbItem>
          <BreadcrumbLink asChild>
            <Link href={{ pathname: '/console/dataset' }}>{titleText}</Link>
          </BreadcrumbLink>
        </BreadcrumbItem>
        {ancestors.map((f, idx) => (
          <span key={f.id} className="contents">
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              {idx === ancestors.length - 1 ? (
                <BreadcrumbPage>{f.name}</BreadcrumbPage>
              ) : (
                <BreadcrumbLink asChild>
                  <Link href={{ pathname: '/console/dataset', query: { folder: f.id } }}>
                    {f.name}
                  </Link>
                </BreadcrumbLink>
              )}
            </BreadcrumbItem>
          </span>
        ))}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
