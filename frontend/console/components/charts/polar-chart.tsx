'use client'

import { useRef, useState, useEffect } from 'react'
import { useTheme } from 'next-themes'

import { chartColors } from '@/components/charts/chartjs-config'
import '@/components/charts/chartjs-config'
import {
  Chart, PolarAreaController, RadialLinearScale, Tooltip, Legend,
} from 'chart.js'
import type { ChartData } from 'chart.js'
import 'chartjs-adapter-moment'

// Import utilities
import { tailwindConfig } from '@/components/utils/utils'

Chart.register(PolarAreaController, RadialLinearScale, Tooltip, Legend)

interface PolarProps {
  data: ChartData
  width: number
  height: number
}

export default function PolarChart({
  data,
  width,
  height
}: PolarProps) {

  const [chart, setChart] = useState<Chart | null>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const legend = useRef<HTMLUListElement>(null)
  const { theme } = useTheme()
  const darkMode = theme === 'dark'
  const { gridColor, textColor, backdropColor, tooltipTitleColor, tooltipBodyColor, tooltipBgColor, tooltipBorderColor } = chartColors

  useEffect(() => {    
    const ctx = canvas.current
    if (!ctx) return
    
    const newChart = new Chart(ctx, {
      type: 'polarArea',
      data: data,
      options: {
        layout: {
          padding: 24,
        },
        scales: {
          r: {
            grid: {
              color: darkMode ? gridColor.dark : gridColor.light,
            },
            ticks: {
              color: darkMode ? textColor.dark : textColor.light,
              backdropColor: darkMode ? backdropColor.dark : backdropColor.light,
            },
          },
        },          
        plugins: {
          legend: {
            display: false,
          },
          tooltip: {
            titleColor: darkMode ? tooltipTitleColor.dark : tooltipTitleColor.light,
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
            li.style.margin = tailwindConfig.theme.margin[1]
            // Button element
            const button = document.createElement('button')
            button.classList.add('btn-xs', 'bg-white', 'dark:bg-gray-700', 'text-gray-500', 'dark:text-gray-400', 'shadow-sm', 'shadow-black/[0.08]', 'rounded-full')
            button.style.opacity = item.hidden ? '.3' : ''
            button.onclick = () => {
              c.toggleDataVisibility(item.index!)
              c.update()
            }
            // Color box
            const box = document.createElement('span')
            box.style.display = 'block'
            box.style.width = tailwindConfig.theme.width[2]
            box.style.height = tailwindConfig.theme.height[2]
            box.style.backgroundColor = item.fillStyle as string
            box.style.borderRadius = tailwindConfig.theme.borderRadius.sm
            box.style.marginRight = tailwindConfig.theme.margin[1]
            box.style.pointerEvents = 'none'
            // Label
            const label = document.createElement('span')
            label.style.display = 'flex'
            label.style.alignItems = 'center'
            const labelText = document.createTextNode(item.text)
            label.appendChild(labelText)
            li.appendChild(button)
            button.appendChild(box)
            button.appendChild(label)
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
      chart.options.scales!.r!.grid!.color = gridColor.dark
      chart.options.scales!.r!.ticks!.color = textColor.dark
      chart.options.scales!.r!.ticks!.backdropColor = backdropColor.dark
      chart.options.plugins!.tooltip!.titleColor = tooltipTitleColor.dark
      chart.options.plugins!.tooltip!.bodyColor = tooltipBodyColor.dark
      chart.options.plugins!.tooltip!.backgroundColor = tooltipBgColor.dark
      chart.options.plugins!.tooltip!.borderColor = tooltipBorderColor.dark
    } else {
      chart.options.scales!.r!.grid!.color = gridColor.light
      chart.options.scales!.r!.ticks!.color = textColor.light
      chart.options.scales!.r!.ticks!.backdropColor = backdropColor.light
      chart.options.plugins!.tooltip!.titleColor = tooltipTitleColor.light
      chart.options.plugins!.tooltip!.bodyColor = tooltipBodyColor.light
      chart.options.plugins!.tooltip!.backgroundColor = tooltipBgColor.light
      chart.options.plugins!.tooltip!.borderColor = tooltipBorderColor.light
    }
    chart.update('none')
  }, [theme])  

  return (
    <div className="grow flex flex-col justify-center">
      <div>
        <canvas ref={canvas} width={width} height={height}></canvas>
      </div>
      <div className="px-5 pt-2 pb-6">
        <ul ref={legend} className="flex flex-wrap justify-center -m-1"></ul>
      </div>
    </div>
  )
}