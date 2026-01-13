// Custom Rich Text Editor for G0 CMS
// Generates HTML following ARTICLE_WRITING_GUIDE.md patterns

class CustomEditor {
  constructor(textareaId, previewId) {
    this.textarea = document.getElementById(textareaId);
    this.preview = document.getElementById(previewId);
    // Note: Chart counter removed - using article-specific prefixes + timestamp for uniqueness
    // This ensures charts work in the Hybrid SSR-SPA pattern where all articles exist in DOM simultaneously
    this.init();
  }

  init() {
    this.setupToolbar();
    this.setupPreview();
    this.setupKeyboardShortcuts();
  }

  setupToolbar() {
    const toolbar = document.createElement("div");
    toolbar.className = "editor-toolbar-custom";
    toolbar.innerHTML = `
            <div class="toolbar-group">
                <button type="button" class="toolbar-btn" data-action="bold" title="Bold (Ctrl+B)">
                    <i class="fa-solid fa-bold"></i>
                </button>
                <button type="button" class="toolbar-btn" data-action="italic" title="Italic (Ctrl+I)">
                    <i class="fa-solid fa-italic"></i>
                </button>
                <button type="button" class="toolbar-btn" data-action="strike" title="Strikethrough">
                    <i class="fa-solid fa-strikethrough"></i>
                </button>
            </div>
            <div class="toolbar-group">
                <button type="button" class="toolbar-btn" data-action="h2" title="Heading 2">
                    H2
                </button>
                <button type="button" class="toolbar-btn" data-action="h3" title="Heading 3">
                    H3
                </button>
            </div>
            <div class="toolbar-group">
                <button type="button" class="toolbar-btn" data-action="paragraph" title="Paragraph">
                    P
                </button>
                <button type="button" class="toolbar-btn" data-action="code" title="Inline Code">
                    <i class="fa-solid fa-code"></i>
                </button>
            </div>
            <div class="toolbar-group">
                <button type="button" class="toolbar-btn" data-action="table" title="Insert Table">
                    <i class="fa-solid fa-table"></i>
                </button>
                <button type="button" class="toolbar-btn" data-action="chart" title="Insert Chart">
                    <i class="fa-solid fa-chart-bar"></i>
                </button>
            </div>
            <div class="toolbar-group">
                <button type="button" class="toolbar-btn" data-action="link" title="Insert Link">
                    <i class="fa-solid fa-link"></i>
                </button>
                <button type="button" class="toolbar-btn" data-action="hr" title="Horizontal Rule">
                    <i class="fa-solid fa-minus"></i>
                </button>
            </div>
        `;

    // Insert toolbar before textarea
    this.textarea.parentNode.insertBefore(toolbar, this.textarea);

    // Attach event listeners
    toolbar.querySelectorAll(".toolbar-btn").forEach((btn) => {
      btn.addEventListener("click", (e) => {
        e.preventDefault();
        const action = btn.getAttribute("data-action");
        this.handleAction(action);
      });
    });
  }

  setupPreview() {
    this.textarea.addEventListener("input", () => {
      this.updatePreview();
    });
  }

  setupKeyboardShortcuts() {
    this.textarea.addEventListener("keydown", (e) => {
      if (e.ctrlKey || e.metaKey) {
        if (e.key === "b") {
          e.preventDefault();
          this.handleAction("bold");
        } else if (e.key === "i") {
          e.preventDefault();
          this.handleAction("italic");
        }
      }
    });
  }

  getSelection() {
    const start = this.textarea.selectionStart;
    const end = this.textarea.selectionEnd;
    const text = this.textarea.value;
    return { start, end, text, selected: text.substring(start, end) };
  }

  setSelection(start, end) {
    this.textarea.setSelectionRange(start, end);
    this.textarea.focus();
  }

  insertAtCursor(before, after = "", replace = false) {
    const sel = this.getSelection();
    let newText, newStart, newEnd;

    if (replace) {
      newText =
        sel.text.substring(0, sel.start) + before + sel.text.substring(sel.end);
      newStart = sel.start + before.length;
      newEnd = newStart;
    } else {
      newText =
        sel.text.substring(0, sel.start) +
        before +
        sel.selected +
        after +
        sel.text.substring(sel.end);
      newStart = sel.start + before.length;
      newEnd = newStart + sel.selected.length;
    }

    this.textarea.value = newText;
    this.setSelection(newStart, newEnd);
    this.updatePreview();
  }

  handleAction(action) {
    const sel = this.getSelection();

    switch (action) {
      case "bold":
        this.insertAtCursor("<strong>", "</strong>");
        break;
      case "italic":
        this.insertAtCursor("<em>", "</em>");
        break;
      case "strike":
        this.insertAtCursor("<s>", "</s>");
        break;
      case "h2":
        if (sel.selected) {
          this.insertAtCursor("<h2>", "</h2>");
        } else {
          this.insertAtCursor("<h2>Heading 2</h2>\n\n", "", true);
        }
        break;
      case "h3":
        if (sel.selected) {
          this.insertAtCursor("<h3>", "</h3>");
        } else {
          this.insertAtCursor("<h3>Heading 3</h3>\n\n", "", true);
        }
        break;
      case "paragraph":
        if (sel.selected) {
          this.insertAtCursor("<p>", "</p>");
        } else {
          this.insertAtCursor("<p>Paragraph text here.</p>\n\n", "", true);
        }
        break;
      case "code":
        this.insertAtCursor("<code>", "</code>");
        break;
      case "link":
        const url = prompt("Enter URL:");
        if (url) {
          const text = sel.selected || "Link Text";
          this.insertAtCursor(
            `<a href="${url}" target="_blank" class="text-[var(--accent)] hover:underline">${text}</a>`,
            ""
          );
        }
        break;
      case "hr":
        this.insertAtCursor(
          '<hr style="border: none; border-top: 1px solid var(--border-primary); margin: 3rem 0;" />\n\n',
          "",
          true
        );
        break;
      case "table":
        this.insertTable();
        break;
      case "chart":
        this.insertChart();
        break;
    }
  }

  insertTable() {
    const rows = prompt("Number of rows:", "3");
    const cols = prompt("Number of columns:", "3");
    if (!rows || !cols) return;

    const r = parseInt(rows);
    const c = parseInt(cols);
    if (isNaN(r) || isNaN(c) || r < 1 || c < 1) return;

    let html =
      '\n<table style="width: 100%; border-collapse: collapse; margin: 2rem 0; border: 1px solid var(--border-primary);">\n';
    html += '<thead style="background-color: var(--bg-secondary);">\n<tr>\n';
    for (let i = 0; i < c; i++) {
      html += `<th style="padding: 0.75rem 1rem; border: 1px solid var(--border-primary); text-align: left; color: var(--text-primary); font-weight: 600;">Header ${
        i + 1
      }</th>\n`;
    }
    html += "</tr>\n</thead>\n<tbody>\n";

    for (let i = 0; i < r; i++) {
      html += '<tr style="border-bottom: 1px solid var(--border-primary);">\n';
      for (let j = 0; j < c; j++) {
        html += `<td style="padding: 0.75rem 1rem; border: 1px solid var(--border-primary); color: var(--text-muted);">Cell ${
          i + 1
        }-${j + 1}</td>\n`;
      }
      html += "</tr>\n";
    }

    html += "</tbody>\n</table>\n\n";

    this.insertAtCursor(html, "", true);
  }

  insertChart() {
    // CRITICAL: In the Hybrid SSR-SPA pattern, all articles exist in DOM simultaneously
    // Canvas IDs must be globally unique across ALL blog posts to prevent conflicts
    const articlePrefix = prompt(
      'Article-specific prefix for chart ID (e.g., "seo", "agi", "cost"):',
      "article"
    );
    if (!articlePrefix) return;

    const chartId = `${articlePrefix}-chart-${Date.now()}`;

    // Chart type selection
    const chartTypeOptions = [
      "bar",
      "line",
      "doughnut",
      "pie",
      "radar",
      "polarArea",
      "bubble",
      "scatter",
    ];
    const chartTypeInput = prompt(
      `Chart type:\n${chartTypeOptions
        .map((t, i) => `${i + 1}. ${t}`)
        .join("\n")}\n\nEnter number or name:`,
      "1"
    );
    let chartType = chartTypeInput;
    const typeNum = parseInt(chartTypeInput);
    if (!isNaN(typeNum) && typeNum >= 1 && typeNum <= chartTypeOptions.length) {
      chartType = chartTypeOptions[typeNum - 1];
    }
    if (!chartType || !chartTypeOptions.includes(chartType.toLowerCase())) {
      chartType = "bar";
    }

    const chartTitle = prompt("Chart title:", "Chart Title");
    if (!chartTitle) return;

    // Labels (X-axis for bar/line, or legend for doughnut/pie)
    const labelsInput = prompt(
      'Labels (comma-separated, e.g., "Jan, Feb, Mar"):',
      "Label 1, Label 2, Label 3"
    );
    const labels = labelsInput
      ? labelsInput.split(",").map((l) => l.trim())
      : ["Label 1", "Label 2", "Label 3"];

    // Data values
    const dataInput = prompt(
      `Data values (comma-separated, e.g., "10, 20, 30"):\n\nShould match ${labels.length} labels`,
      labels.map((_, i) => (i + 1) * 10).join(", ")
    );
    const dataValues = dataInput
      ? dataInput.split(",").map((v) => parseFloat(v.trim()) || 0)
      : labels.map((_, i) => (i + 1) * 10);

    // Dataset label
    const datasetLabel = prompt("Dataset label (series name):", "Dataset");

    // Y-axis label (only for bar/line charts)
    let yAxisLabel = "";
    if (["bar", "line"].includes(chartType)) {
      yAxisLabel = prompt("Y-axis label:", "Value") || "Value";
    }

    // Chart caption
    const caption = prompt(
      "Chart caption/description:",
      "Description of what the chart shows."
    );

    // Generate color arrays for multi-color charts (doughnut, pie, etc.)
    const needsMultiColor = ["doughnut", "pie", "polarArea"].includes(
      chartType
    );

    // Color palette for multi-color charts
    const colorPalette = [
      "rgba(138, 180, 248, 0.8)", // primary blue (dark theme)
      "rgba(123, 200, 150, 0.8)", // secondary green
      "rgba(255, 159, 64, 0.8)", // orange
      "rgba(255, 99, 132, 0.8)", // pink
      "rgba(153, 102, 255, 0.8)", // purple
      "rgba(54, 162, 235, 0.8)", // light blue
      "rgba(255, 206, 86, 0.8)", // yellow
      "rgba(75, 192, 192, 0.8)", // teal
    ];

    const lightColorPalette = [
      "rgba(79, 70, 229, 0.6)", // indigo (light theme)
      "rgba(22, 163, 74, 0.6)", // green
      "rgba(245, 158, 11, 0.6)", // amber
      "rgba(239, 68, 68, 0.6)", // red
      "rgba(139, 92, 246, 0.6)", // violet
      "rgba(59, 130, 246, 0.6)", // blue
      "rgba(234, 179, 8, 0.6)", // yellow
      "rgba(20, 184, 166, 0.6)", // teal
    ];

    let backgroundColorCode, borderColorCode;

    if (needsMultiColor) {
      // Generate arrays that will be theme-aware at runtime
      const darkColors = labels.map(
        (_, i) => colorPalette[i % colorPalette.length]
      );
      const lightColors = labels.map(
        (_, i) => lightColorPalette[i % lightColorPalette.length]
      );
      const darkBorders = darkColors.map((c) => c.replace("0.8", "1"));
      const lightBorders = lightColors.map((c) => c.replace("0.6", "1"));

      backgroundColorCode = `(isDark ? ${JSON.stringify(
        darkColors
      )} : ${JSON.stringify(lightColors)})`;
      borderColorCode = `(isDark ? ${JSON.stringify(
        darkBorders
      )} : ${JSON.stringify(lightBorders)})`;
    } else {
      // Single color for bar/line charts (theme-aware)
      backgroundColorCode = "colors.primary + '80'";
      borderColorCode = "colors.primary";
    }

    // Build scales config (only for bar/line charts)
    let scalesConfig = "";
    if (["bar", "line"].includes(chartType)) {
      scalesConfig = `
                scales: {
                    y: {
                        beginAtZero: true,
                        title: { display: true, text: '${yAxisLabel}', color: colors.text },
                        grid: { color: colors.grid },
                        ticks: { color: colors.text }
                    },
                    x: {
                        grid: { display: false },
                        ticks: { color: colors.text }
                    }
                }`;
    } else {
      scalesConfig = "scales: {}"; // No scales for doughnut/pie charts
    }

    const html = `
<div style="position: relative; height:400px; width:100%; margin: 32px 0;">
    <canvas id="${chartId}"></canvas>
</div>
<script>
(function() {
    let chartInstance = null;
    
    function initChart() {
        if (chartInstance) {
            chartInstance.destroy();
        }
        
        const currentTheme = document.documentElement.getAttribute('data-theme') || 'dark';
        const isDark = currentTheme === 'dark';
        
        const colors = {
            primary: isDark ? '#8AB4F8' : '#4f46e5',
            secondary: isDark ? '#7bc896' : '#16a34a',
            text: isDark ? '#e5e5e5' : '#2D2D2D',
            grid: isDark ? '#404040' : '#d0d0d0'
        };

        const ctx = document.getElementById('${chartId}');
        if (!ctx) return;

        chartInstance = new Chart(ctx.getContext('2d'), {
            type: '${chartType}',
            data: {
                labels: ${JSON.stringify(labels)},
                datasets: [{
                    label: '${datasetLabel}',
                    data: ${JSON.stringify(dataValues)},
                    backgroundColor: ${backgroundColorCode},
                    borderColor: ${borderColorCode},
                    borderWidth: 2
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                plugins: {
                    title: {
                        display: true,
                        text: '${chartTitle}',
                        color: colors.text,
                        font: { size: 16, weight: '600' }
                    },
                    tooltip: {
                        enabled: true,
                        backgroundColor: isDark ? 'rgba(0,0,0,0.8)' : 'rgba(255,255,255,0.95)',
                        titleColor: colors.text,
                        bodyColor: colors.text,
                        borderColor: colors.grid,
                        borderWidth: 1
                    },
                    legend: {
                        position: 'top',
                        labels: {
                            usePointStyle: true,
                            boxWidth: 10,
                            color: colors.text,
                            font: { size: 13 }
                        }
                    }
                },
                ${scalesConfig}
            }
        });
    }
    
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initChart);
    } else {
        initChart();
    }
    
    document.addEventListener('themeChanged', initChart);
})();
</script>

<p class="text-center text-sm text-[var(--text-secondary)] mt-2 mb-8">
  <em>Fig 1. ${caption || "Description of what the chart shows."}</em>
</p>

`;

    this.insertAtCursor(html, "", true);
  }

  updatePreview() {
    if (this.preview) {
      const content = this.textarea.value;
      this.preview.innerHTML = content;

      // Execute inline scripts (for Chart.js initialization)
      // Note: Scripts in innerHTML don't execute automatically for security
      const scripts = this.preview.querySelectorAll("script");
      scripts.forEach((oldScript) => {
        const newScript = document.createElement("script");
        // Copy attributes
        Array.from(oldScript.attributes).forEach((attr) => {
          newScript.setAttribute(attr.name, attr.value);
        });
        // Copy content
        newScript.textContent = oldScript.textContent;
        // Replace and execute
        oldScript.parentNode.replaceChild(newScript, oldScript);
      });

      // Trigger Prism syntax highlighting for code blocks in preview
      if (typeof Prism !== "undefined") {
        Prism.highlightAllUnder(this.preview);
      }
    }
  }
}

// Initialize editor when DOM is ready
document.addEventListener("DOMContentLoaded", () => {
  const textarea = document.getElementById("raw-injector");
  const preview = document.getElementById("live-preview");
  if (textarea && preview) {
    window.customEditor = new CustomEditor("raw-injector", "live-preview");
  }
});
