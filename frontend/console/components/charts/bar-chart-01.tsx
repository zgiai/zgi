'use client'

import { useRef, useState, useEffect } from 'react'
import { useTheme } from 'next-themes'

import { chartColors } from '@/components/charts/chartjs-config'
import '@/components/charts/chartjs-config'
import {
  Chart, BarController, BarElement, LinearScale, TimeScale, Tooltip, Legend,
} from 'chart.js'
import type { ChartData } from 'chart.js'
import 'chartjs-adapter-moment'

// Import utilities
import { tailwindConfig, formatValue } from '@/components/utils/utils'

Chart.register(BarController, BarElement, LinearScale, TimeScale, Tooltip, Legend)

interface BarChartProps {
  data: ChartData
  width: number
  height: number
}

export default function BarChart01({
  data,
  width,
  height
}: BarChartProps) {
  
  const [chart, setChart] = useState<Chart | null>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const legend = useRef<HTMLUListElement>(null)
  const { theme } = useTheme()
  const darkMode = theme === 'dark'
  const { textColor, gridColor, tooltipBodyColor, tooltipBgColor, tooltipBorderColor } = chartColors

  useEffect(() => {    
    const ctx = canvas.current
    if (!ctx) return
    
    const newChart = new Chart(ctx, {
      type: 'bar',
      data: data,
      options: {
        layout: {
          padding: {
            top: 12,
            bottom: 16,
            left: 20,
            right: 20,
          },
        },
        scales: {
          y: {
            border: {
              display: false,
            },
            ticks: {
              maxTicksLimit: 5,
              callback: (value) => formatValue(+value),
              color: darkMode ? textColor.dark : textColor.light,
            },
            grid: {
              color: darkMode ? gridColor.dark : gridColor.light,
            },
          },
          x: {
            type: 'time',
            time: {
              parser: 'MM-DD-YYYY',
              unit: 'month',
              displayFormats: {
                month: 'MMM YY',
              },
            },
            border: {
              display: false,
            },
            grid: {
              display: false,
            },
            ticks: {
              color: darkMode ? textColor.dark : textColor.light,
            },
          },
        },
        plugins: {
          legend: {
            display: false,
          },
          tooltip: {
            callbacks: {
              title: () => '', // Disable tooltip title
              label: (context) => formatValue(context.parsed.y),
            },
            bodyColor: darkMode ? tooltipBodyColor.dark : tooltipBodyColor.light,
            backgroundColor: darkMode ? tooltipBgColor.dark : tooltipBgColor.light,
            borderColor: darkMode ? tooltipBorderColor.dark : tooltipBorderColor.light,                  
          },
        },
        interaction: {
          intersect: false,
          mode: 'nearest',
        },
        animation: {
          duration: 500,
        },
        maintainAspectRatio: false,
        resizeDelay: 200,
      },
      plugins: [{
        id: 'htmlLegend',
        afterUpdate(c, args, options) {
          const ul = legend.current
          if (!ul) return
          // Remove old legend items
          while (ul.firstChild) {
            ul.firstChild.remove()
          }
          // Reuse the built-in legendItems generator          
          const items = c.options.plugins?.legend?.labels?.generateLabels?.(c)
          items?.forEach((item) => {            
            const li = document.createElement('li')
            // Button element
            const button = document.createElement('button')
            button.style.display = 'inline-flex'
            button.style.alignItems = 'center'
            button.style.opacity = item.hidden ? '.3' : ''
            button.onclick = () => {
              c.setDatasetVisibility(item.datasetIndex!, !c.isDatasetVisible(item.datasetIndex!))
              c.update()
            }
            // Color box
            const box = document.createElement('span')
            box.style.display = 'block'
            box.style.width = tailwindConfig.theme.width[3]
            box.style.height = tailwindConfig.theme.height[3]
            box.style.borderRadius = tailwindConfig.theme.borderRadius.full
            box.style.marginRight = tailwindConfig.theme.margin[2]
            box.style.borderWidth = '3px'
            box.style.borderColor = item.fillStyle as string
            box.style.pointerEvents = 'none'
            // Label
            const labelContainer = document.createElement('span')
            labelContainer.style.display = 'flex'
            labelContainer.style.alignItems = 'center'
            const value = document.createElement('span')
            value.classList.add('text-gray-800', 'dark:text-gray-100')
            value.style.fontSize = tailwindConfig.theme.fontSize['3xl'][0]
            value.style.lineHeight = tailwindConfig.theme.fontSize['3xl'][1].lineHeight
            value.style.fontWeight = tailwindConfig.theme.fontWeight.bold
            value.style.marginRight = tailwindConfig.theme.margin[2]
            value.style.pointerEvents = 'none'
            const label = document.createElement('span')
            label.classList.add('text-gray-500', 'dark:text-gray-400')
            label.style.fontSize = tailwindConfig.theme.fontSize.sm[0]
            label.style.lineHeight = tailwindConfig.theme.fontSize.sm[1].lineHeight
            // @ts-ignore
            const theValue: number = c.data.datasets[item.datasetIndex!].data.reduce((a, b) => a + b, 0)
            const valueText = document.createTextNode(formatValue(theValue))
            const labelText = document.createTextNode(item.text)
            value.appendChild(valueText)
            label.appendChild(labelText)
            li.appendChild(button)
            button.appendChild(box)
            button.appendChild(labelContainer)
            labelContainer.appendChild(value)
            labelContainer.appendChild(label)
            ul.appendChild(li)
          })
        },
      }],
    })
    setChart(newChart)
    return () => newChart.destroy()
  }, [])

  useEffect(() => {
    if (!chart) return

    if (darkMode) {
      chart.options.scales!.x!.ticks!.color = textColor.dark
      chart.options.scales!.y!.ticks!.color = textColor.dark
      chart.options.scales!.y!.grid!.color = gridColor.dark
      chart.options.plugins!.tooltip!.bodyColor = tooltipBodyColor.dark
      chart.options.plugins!.tooltip!.backgroundColor = tooltipBgColor.dark
      chart.options.plugins!.tooltip!.borderColor = tooltipBorderColor.dark          
    } else {
      chart.options.scales!.x!.ticks!.color = textColor.light
      chart.options.scales!.y!.ticks!.color = textColor.light
      chart.options.scales!.y!.grid!.color = gridColor.light
      chart.options.plugins!.tooltip!.bodyColor = tooltipBodyColor.light
      chart.options.plugins!.tooltip!.backgroundColor = tooltipBgColor.light
      chart.options.plugins!.tooltip!.borderColor = tooltipBorderColor.light  
    }
    chart.update('none')
  }, [theme])    

  return (
    <>
      <div className="px-5 py-3">
        <ul ref={legend} className="flex flex-wrap gap-x-4"></ul>
      </div>
      <div className="grow">
        <canvas ref={canvas} width={width} height={height}></canvas>
      </div>
    </>
  )
}