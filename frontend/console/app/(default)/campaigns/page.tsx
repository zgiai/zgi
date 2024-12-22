export const metadata = {
  title: 'Campaigns - Mosaic',
  description: 'Page description',
}

import SearchForm from '@/components/search-form'
import FilterButton from '@/components/dropdown-filter'
import CampaignCard from './campaign-card'
import PaginationNumeric from '@/components/pagination-numeric'

import Image01 from '@/public/images/user-28-01.jpg'
import Image02 from '@/public/images/user-28-02.jpg'
import Image03 from '@/public/images/user-28-03.jpg'
import Image04 from '@/public/images/user-28-04.jpg'
import Image05 from '@/public/images/user-28-05.jpg'
import Image06 from '@/public/images/user-28-06.jpg'
import Image07 from '@/public/images/user-28-07.jpg'
import Image08 from '@/public/images/user-28-08.jpg'
import Image09 from '@/public/images/user-28-09.jpg'
import Image10 from '@/public/images/user-28-10.jpg'
import Image11 from '@/public/images/user-28-11.jpg'
import Image12 from '@/public/images/user-28-12.jpg'

export default function Campaigns() {

  // Some dummy campaigns data
  const campaigns = [
    {
      id: 0,
      category: '1',
      members: [
        {
          name: 'User 01',
          image: Image01,
          link: '#0'
        },
        {
          name: 'User 02',
          image: Image02,
          link: '#0'
        },
        {
          name: 'User 03',
          image: Image03,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'One-Time'
    },
    {
      id: 1,
      category: '2',
      members: [
        {
          name: 'User 04',
          image: Image04,
          link: '#0'
        },
        {
          name: 'User 05',
          image: Image05,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'Off-Track'
    },
    {
      id: 3,
      category: '3',
      members: [
        {
          name: 'User 07',
          image: Image07,
          link: '#0'
        },
        {
          name: 'User 08',
          image: Image08,
          link: '#0'
        },
        {
          name: 'User 09',
          image: Image09,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'At Risk'
    },
    {
      id: 4,
      category: '1',
      members: [
        {
          name: 'User 10',
          image: Image10,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'At Risk'
    },
    {
      id: 5,
      category: '4',
      members: [
        {
          name: 'User 11',
          image: Image11,
          link: '#0'
        },
        {
          name: 'User 05',
          image: Image05,
          link: '#0'
        },
        {
          name: 'User 12',
          image: Image12,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'One-Time'
    },
    {
      id: 6,
      category: '2',
      members: [
        {
          name: 'User 07',
          image: Image07,
          link: '#0'
        },
        {
          name: 'User 04',
          image: Image04,
          link: '#0'
        },
        {
          name: 'User 11',
          image: Image11,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'At Risk'
    },
    {
      id: 7,
      category: '4',
      members: [
        {
          name: 'User 01',
          image: Image01,
          link: '#0'
        },
        {
          name: 'User 02',
          image: Image02,
          link: '#0'
        },
        {
          name: 'User 06',
          image: Image06,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'One-Time'
    },
    {
      id: 8,
      category: '1',
      members: [
        {
          name: 'User 09',
          image: Image09,
          link: '#0'
        },
        {
          name: 'User 01',
          image: Image01,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'Off-Track'
    },
    {
      id: 9,
      category: '3',
      members: [
        {
          name: 'User 07',
          image: Image07,
          link: '#0'
        },
        {
          name: 'User 08',
          image: Image08,
          link: '#0'
        },
        {
          name: 'User 09',
          image: Image09,
          link: '#0'
        },
        {
          name: 'User 06',
          image: Image06,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'One-Time'
    },
    {
      id: 10,
      category: '4',
      members: [
        {
          name: 'User 06',
          image: Image06,
          link: '#0'
        },
        {
          name: 'User 11',
          image: Image11,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'Off-Track'
    },
    {
      id: 11,
      category: '2',
      members: [
        {
          name: 'User 05',
          image: Image05,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'Off-Track'
    },
    {
      id: 12,
      category: '3',
      members: [
        {
          name: 'User 07',
          image: Image07,
          link: '#0'
        },
        {
          name: 'User 08',
          image: Image08,
          link: '#0'
        },
        {
          name: 'User 09',
          image: Image09,
          link: '#0'
        },
      ],
      title: 'Monitor progress in Real Time Value',
      link: '#0',
      content: 'Lorem ipsum is placeholder text commonly used in the graphic, print, and publishing industries for previewing layouts.',
      dates: {
        from: 'Jan 20',
        to: 'Jan 27'
      },
      type: 'At Risk'
    },
  ]

  return (
    <div className="px-4 sm:px-6 lg:px-8 py-8 w-full max-w-[96rem] mx-auto">

      {/* Page header */}
      <div className="sm:flex sm:justify-between sm:items-center mb-8">

        {/* Left: Title */}
        <div className="mb-4 sm:mb-0">
          <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Campaigns</h1>
        </div>

        {/* Right: Actions */}
        <div className="grid grid-flow-col sm:auto-cols-max justify-start sm:justify-end gap-2">
          {/* Search form */}
          <SearchForm />
          {/* Filter button */}
          <FilterButton align="right" />
          {/* Create campaign button */}
          <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">
            <svg className="fill-current shrink-0 xs:hidden" width="16" height="16" viewBox="0 0 16 16">
              <path d="M15 7H9V1c0-.6-.4-1-1-1S7 .4 7 1v6H1c-.6 0-1 .4-1 1s.4 1 1 1h6v6c0 .6.4 1 1 1s1-.4 1-1V9h6c.6 0 1-.4 1-1s-.4-1-1-1z" />
            </svg>
            <span className="max-xs:sr-only">Create Campaign</span>
          </button>
        </div>

      </div>

      {/* Cards */}
      <div className="grid grid-cols-12 gap-6">
        {campaigns.map(campaign => (
          <CampaignCard
            key={campaign.id}
            campaign={campaign} />
        ))}
      </div>

      {/* Pagination */}
      <div className="mt-8">
        <PaginationNumeric />
      </div>

    </div>
  )
}
