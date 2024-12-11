from datetime import datetime
from typing import List, Dict
import io
from reportlab.lib import colors
from reportlab.lib.pagesizes import letter
from reportlab.lib.styles import getSampleStyleSheet, ParagraphStyle
from reportlab.lib.units import inch
from reportlab.platypus import SimpleDocTemplate, Table, TableStyle, Paragraph, Spacer
from reportlab.graphics.shapes import Drawing
from reportlab.graphics.charts.linecharts import HorizontalLineChart

class BillingReportGenerator:
    def __init__(self):
        self.styles = getSampleStyleSheet()
        self._setup_custom_styles()

    def _setup_custom_styles(self):
        """Setup custom paragraph styles"""
        self.styles.add(ParagraphStyle(
            name='CustomTitle',
            parent=self.styles['Heading1'],
            fontSize=24,
            spaceAfter=30
        ))
        self.styles.add(ParagraphStyle(
            name='SectionHeader',
            parent=self.styles['Heading2'],
            fontSize=14,
            spaceAfter=12
        ))

    def generate_pdf(
        self,
        application_name: str,
        start_date: datetime,
        end_date: datetime,
        daily_costs: List[Dict],
        trend_data: Dict
    ) -> bytes:
        """Generate PDF billing report"""
        buffer = io.BytesIO()
        doc = SimpleDocTemplate(
            buffer,
            pagesize=letter,
            rightMargin=72,
            leftMargin=72,
            topMargin=72,
            bottomMargin=72
        )

        # Build the document content
        story = []
        
        # Title
        title = Paragraph(
            f"Billing Report - {application_name}",
            self.styles['CustomTitle']
        )
        story.append(title)
        
        # Date Range
        date_range = Paragraph(
            f"Period: {start_date.date()} to {end_date.date()}",
            self.styles['Normal']
        )
        story.append(date_range)
        story.append(Spacer(1, 20))

        # Summary Section
        story.append(Paragraph("Summary", self.styles['SectionHeader']))
        total_cost = sum(day['total_cost'] for day in daily_costs)
        summary_data = [
            ['Total Cost', f"${total_cost:.2f}"],
            ['Number of Days', str(len(daily_costs))],
            ['Average Daily Cost', f"${(total_cost/len(daily_costs)):.2f}"]
        ]
        summary_table = Table(summary_data, colWidths=[2*inch, 2*inch])
        summary_table.setStyle(TableStyle([
            ('GRID', (0, 0), (-1, -1), 1, colors.black),
            ('BACKGROUND', (0, 0), (0, -1), colors.lightgrey),
            ('ALIGN', (0, 0), (-1, -1), 'CENTER'),
            ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
            ('PADDING', (0, 0), (-1, -1), 6),
        ]))
        story.append(summary_table)
        story.append(Spacer(1, 20))

        # Cost Trend Chart
        story.append(Paragraph("Daily Cost Trend", self.styles['SectionHeader']))
        drawing = Drawing(400, 200)
        lc = HorizontalLineChart()
        lc.x = 50
        lc.y = 50
        lc.height = 125
        lc.width = 300
        
        # Prepare data for chart
        costs = [day['total_cost'] for day in daily_costs]
        lc.data = [costs]
        
        # Configure chart
        lc.lines[0].strokeColor = colors.blue
        lc.lines[0].strokeWidth = 2
        drawing.add(lc)
        story.append(drawing)
        story.append(Spacer(1, 20))

        # Daily Costs Table
        story.append(Paragraph("Daily Cost Details", self.styles['SectionHeader']))
        table_data = [['Date', 'Total Cost'] + list(daily_costs[0]['model_costs'].keys())]
        for day in daily_costs:
            row = [
                day['date'].strftime('%Y-%m-%d'),
                f"${day['total_cost']:.2f}"
            ]
            row.extend([f"${day['model_costs'].get(model, 0):.2f}" for model in table_data[0][2:]])
            table_data.append(row)

        # Create table
        cost_table = Table(table_data, colWidths=[1.2*inch] + [1*inch]*(len(table_data[0])-1))
        cost_table.setStyle(TableStyle([
            ('GRID', (0, 0), (-1, -1), 1, colors.black),
            ('BACKGROUND', (0, 0), (-1, 0), colors.lightgrey),
            ('ALIGN', (0, 0), (-1, -1), 'CENTER'),
            ('FONTNAME', (0, 0), (-1, 0), 'Helvetica-Bold'),
            ('PADDING', (0, 0), (-1, -1), 6),
        ]))
        story.append(cost_table)

        # Build PDF
        doc.build(story)
        buffer.seek(0)
        return buffer.getvalue()
