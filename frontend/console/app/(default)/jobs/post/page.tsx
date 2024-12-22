export const metadata = {
  title: 'Job Post - Mosaic',
  description: 'Page description',
}

import Link from 'next/link'
import Image from 'next/image'
import JobsItem from '../jobs-item'
import Image03 from '@/public/images/company-icon-03.svg'
import Image04 from '@/public/images/company-icon-07.svg'
import Image05 from '@/public/images/company-icon-08.svg'
import Image06 from '@/public/images/company-icon-01.svg'

export default function Jobs() {

  // Some dummy jobs data
  const jobs = [
    {
      id: 3,
      image: Image03,
      company: 'Company 04',
      role: 'Senior Software Engineer Backend',
      link: '/jobs/post',
      details: 'Full-time / Remote / Anywhere',
      date: 'Jan 7',
      type: 'New',
      fav: false,
    },
    {
      id: 4,
      image: Image04,
      company: 'Company 05',
      role: 'React.js Software Developer',
      link: '/jobs/post',
      details: 'Full-time / Remote / London, UK',
      date: 'Jan 6',
      type: 'New',
      fav: true,
    },
    {
      id: 5,
      image: Image05,
      company: 'Company 06',
      role: 'Senior Full Stack Rails Developer',
      link: '/jobs/post',
      details: 'Part-time / Remote / Milan, IT',
      date: 'Jan 6',
      type: 'New',
      fav: false,
    },
  ]

  return (
    <div className="px-4 sm:px-6 lg:px-8 py-8 w-full">
      {/* Page content */}
      <div className="max-w-5xl mx-auto flex flex-col lg:flex-row lg:space-x-8 xl:space-x-16">
        {/* Content */}
        <div>
          <div className="mb-6">
            <Link className="btn-sm px-3 bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300" href="/jobs">
              <svg className="fill-current text-gray-400 dark:text-gray-500 mr-2" width="7" height="12" viewBox="0 0 7 12">
                <path d="M5.4.6 6.8 2l-4 4 4 4-1.4 1.4L0 6z" />
              </svg>
              <span>Back To Jobs</span>
            </Link>
          </div>
          <div className="text-sm text-gray-500 dark:text-gray-400 italic mb-2">Posted Jan 6, 2024</div>
          <header className="mb-4">
            {/* Title */}
            <h1 className="text-2xl md:text-3xl text-gray-800 dark:text-gray-100 font-bold">Senior Client Engineer (React &amp; React Native)</h1>
          </header>

          {/* Company information (mobile) */}
          <div className="bg-white dark:bg-gray-800 p-5 shadow-lg rounded-xl mb-6 lg:hidden">
            <div className="text-center mb-6">
              <div className="inline-flex mb-3">
                <Image className="w-16 h-16 rounded-full" src={Image06} width={64} height={64} alt="Company 01" />
              </div>
              <div className="text-lg font-bold text-gray-800 dark:text-gray-100 mb-1">Revolut Ltd</div>
              <div className="text-sm text-gray-500 dark:text-gray-400 italic">179 Jobs Posted</div>
            </div>
            <div className="space-y-2 sm:flex sm:space-y-0 sm:space-x-2">
              <button className="btn w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">Apply Today -&gt;</button>
              <button className="btn w-full border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300">Company Profile</button>
            </div>
          </div>

          {/* Tags */}
          <div className="mb-6">
            <div className="flex flex-wrap items-center -m-1">
              <div className="m-1">
                <a className="btn-xs text-xs border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300 px-2.5 py-1 rounded-full shadow-none" href="#0">
                  Full-time
                </a>
              </div>
              <div className="m-1">
                <a className="btn-xs text-xs border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300 px-2.5 py-1 rounded-full shadow-none" href="#0">
                  Senior Engineer
                </a>
              </div>
              <div className="m-1">
                <a className="btn-xs text-xs border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300 px-2.5 py-1 rounded-full shadow-none" href="#0">
                  Remote
                </a>
              </div>
            </div>
          </div>

          <hr className="my-6 border-t border-gray-100 dark:border-gray-700/60" />

          {/* The Role */}
          <div>
            <h2 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-2">The Role</h2>
            <div className="space-y-6">
              <p>In the world of AI, behavioural predictions are leading the charge to better machine learning.</p>
              <p>
                There is so much happening in the AI space. Advances in the economic sectors have seen automated business practices rapidly
                increasing economic value. While the realm of the human sciences has used the power afforded by computational capabilities to
                solve many human based dilemmas. Even the art scene has adopted carefully selected ML applications to usher in the technological
                movement.
              </p>
              <p>
                Join us every second Wednesday as we host an open discussion about the amazing things happening in the world of AI and machine
                learning. Feel free to share your experiences, ask questions, ponder the possibilities, or just listen as we explore new topics
                and revisit old ones.
              </p>
            </div>
          </div>

          <hr className="my-6 border-t border-gray-100 dark:border-gray-700/60" />

          {/* About You */}
          <div>
            <h2 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-2">About You</h2>
            <div className="space-y-6">
              <p>
                You love building great software. Your work could be supporting new feature development, migrating existing features, and
                creating other mobile and web solutions for customers. You'll have a primary focus on frontend development using Javascript. Our
                client's tech stack is JavaScript, primarily using React. A strong understanding of JS core (ES2019+) is required, with some
                exposure in Java as back-end technology. We use modern tools, which means you'll have the opportunity to work with Webpack,
                Redux, Apollo, Styled Components, and much more.
              </p>
              <p>
                You love learning. Engineering is an ever-evolving world. You enjoy playing with new tech and exploring areas that you might not
                have experience with yet. You are self-driven, self-learner willing to share knowledge and participate actively in your
                community.
              </p>
              <p>
                Having overlap with your team is critical when working in a global remote team. Modus requires all team members to overlap with
                EST morning hours daily. In addition, reliable high speed internet is a must.
              </p>
            </div>
          </div>

          <hr className="my-6 border-t border-gray-100 dark:border-gray-700/60" />

          {/* Things You Might Do */}
          <div>
            <h2 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-2">Things You Might Do</h2>
            <div className="space-y-6">
              <p>
                We are a fast-growing, and remote-first company, so you'll likely get experience on many different projects across the
                organization. That said, here are some things you'll probably do:
              </p>
              <ul className="list-disc list-inside space-y-1">
                <li>Give back to the community via open source and blog posts</li>
                <li>
                  Travel and meet great people- as part of our remote-first lifestyle, it's important that we come together as needed to work
                  together, meet each other in person and have fun together. Please keep that in mind when you apply.
                </li>
                <li>
                  Teach and be taught: Modus creates active teams that work in internal and external projects together, giving opportunities to
                  stay relevant with the latest technologies and learning from experts worldwide
                </li>
                <li>Interact directly with internal and external clients to represent Modus and its values</li>
              </ul>
            </div>
          </div>

          {/* Apply section */}
          <div className="mt-6">
            <p className="font-medium italic mb-6">Do you have what it takes?</p>
            <div className="flex justify-between items-center">
              {/* Apply button */}
              <button className="btn bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white whitespace-nowrap">Apply Today -&gt;</button>
              {/* Share */}
              <div className="flex items-center">
                <div className="text-sm text-gray-500 dark:text-gray-400 italic mr-4">Share:</div>
                <div className="flex items-center space-x-3">
                  <button className="text-gray-400 dark:text-gray-500 hover:text-violet-500 dark:hover:text-violet-500">
                    <span className="sr-only">Share on X</span>
                    <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16" xmlns="http://www.w3.org/2000/svg">
                      <path d="M16 15h-4.938l-3.865-4.892L2.77 15H.316L6.05 8.658 0 1h5.063l3.496 4.476L12.601 1h2.454L9.697 6.932 16 15Zm-4.26-1.421h1.36L4.323 2.347H2.865l8.875 11.232Z" />
                    </svg>
                  </button>
                  <button className="text-gray-400 dark:text-gray-500 hover:text-violet-500 dark:hover:text-violet-500">
                    <span className="sr-only">Share on Facebook</span>
                    <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16" xmlns="http://www.w3.org/2000/svg">
                      <path d="M6.023 16 6 9H3V6h3V4c0-2.7 1.672-4 4.08-4 1.153 0 2.144.086 2.433.124v2.821h-1.67c-1.31 0-1.563.623-1.563 1.536V6H13l-1 3H9.28v7H6.023Z" />
                    </svg>
                  </button>
                  <button className="text-gray-400 dark:text-gray-500 hover:text-violet-500 dark:hover:text-violet-500">
                    <span className="sr-only">Share on Linkedin</span>
                    <svg className="fill-current" width="16" height="16" viewBox="0 0 16 16" xmlns="http://www.w3.org/2000/svg">
                      <path d="M0 1.146C0 .514.53 0 1.182 0h13.635C15.471 0 16 .513 16 1.146v13.708c0 .633-.53 1.146-1.183 1.146H1.182C.53 16 0 15.487 0 14.854V1.146ZM4.862 13.39V6.187H2.468v7.203h2.394ZM3.666 5.203c.834 0 1.354-.553 1.354-1.244-.016-.707-.52-1.245-1.338-1.245-.82 0-1.355.538-1.355 1.245 0 .691.52 1.244 1.323 1.244h.015Zm2.522 8.187h2.394V9.368c0-.215.015-.43.078-.584.173-.43.567-.876 1.229-.876.866 0 1.213.66 1.213 1.629v3.853h2.394V9.26c0-2.213-1.181-3.242-2.756-3.242-1.292 0-1.86.722-2.174 1.213h.016V6.187H6.188c.03.676 0 7.203 0 7.203Z" />
                    </svg>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <hr className="my-6 border-t border-gray-100 dark:border-gray-700/60" />

          {/* Related Jobs */}
          <div>
            <h2 className="text-xl leading-snug text-gray-800 dark:text-gray-100 font-bold mb-6">Related Jobs</h2>
            <div className="space-y-2 mt-6">
              {jobs.map(job => (
                <JobsItem
                  key={job.id}
                  job={job} />
              ))}
            </div>
          </div>
        </div>

        {/* Sidebar */}
        <div className="hidden lg:block space-y-4">

          {/* Company information (desktop) */}
          <div className="bg-white dark:bg-gray-800 p-5 shadow-sm rounded-xl lg:w-[18rem] xl:w-[20rem]">
            <div className="text-center mb-6">
              <div className="inline-flex mb-3">
                <Image className="w-16 h-16 rounded-full" src={Image06} width={64} height={64} alt="Company 01" />
              </div>
              <div className="text-lg font-bold text-gray-800 dark:text-gray-100 mb-1">Revolut Ltd</div>
              <div className="text-sm text-gray-500 dark:text-gray-400 italic">179 Jobs Posted</div>
            </div>
            <div className="space-y-2">
              <button className="btn w-full bg-gray-900 text-gray-100 hover:bg-gray-800 dark:bg-gray-100 dark:text-gray-800 dark:hover:bg-white">Apply Today -&gt;</button>
              <button className="btn w-full border-gray-200 dark:border-gray-700/60 hover:border-gray-300 dark:hover:border-gray-600 text-gray-800 dark:text-gray-300">Company Profile</button>
            </div>
          </div>

        </div>

      </div>

    </div>
  )
}
