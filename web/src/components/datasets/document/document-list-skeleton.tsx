import { Skeleton } from '@/components/ui/skeleton';
import { Table, TableBody, TableCell, TableRow } from '@/components/ui/table';

interface DocumentListSkeletonProps {
  rows?: number;
}

export function DocumentListSkeleton({ rows = 5 }: DocumentListSkeletonProps) {
  return (
    <div className="rounded-md border">
      <Table>
        <TableBody>
          {Array.from({ length: rows }).map((_, rowIndex) => (
            <TableRow key={`skeleton-row-${rowIndex}`}> 
              {Array.from({ length: 7 }).map((__, cellIndex) => (
                <TableCell key={`skeleton-cell-${rowIndex}-${cellIndex}`}> 
                  <Skeleton className="h-4 w-full" />
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
} 