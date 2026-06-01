<script>
  import { onMount } from 'svelte';
  import { 
    LoadConfig, 
    SaveConfig, 
    RunCommandAsync, 
    RunCommandSync, 
    SelectImage, 
    CopyImageToConfig,
    GetImageBase64,
    ListConfigImages
  } from '../wailsjs/go/main/App.js';

  // State
  let config = { rows: 2, cols: 4, pages: [] };
  let activeTab = 'deck'; // 'deck' | 'config'
  let selectedSlot = null; // index of slot currently selected for editing (0 to rows*cols-1)
  let base64Cache = {}; // button.id -> base64 data URL
  let currentPage = 0; // Current active page index (0-based)
  const totalPages = 5; // Support 5 pages of buttons
  
  // Editor form state
  let editId = '';
  let editLabel = '';
  let editCommand = '';
  let editBgImage = '';
  let editBgColor = '#1f2937';
  let editFontColor = '#ffffff';
  let editFontSize = 0; // -2 to +2 font size modifier
  let existingImages = []; // List of existing image absolute paths

  async function loadExistingImages() {
    try {
      existingImages = await ListConfigImages() || [];
    } catch (err) {
      console.error("Failed to load existing images:", err);
    }
  }

  function getFriendlyImageName(path) {
    if (!path) return '';
    const parts = path.split('/');
    const filename = parts[parts.length - 1];
    // Find the first underscore to strip the timestamp prefix
    const firstUnderscore = filename.indexOf('_');
    if (firstUnderscore !== -1) {
      return filename.slice(firstUnderscore + 1);
    }
    return filename;
  }

  function getFontSizeClass(offset) {
    switch (offset) {
      case -2: return 'text-xs sm:text-xs';
      case -1: return 'text-sm sm:text-sm';
      case 1:  return 'text-xl sm:text-2xl';
      case 2:  return 'text-2xl sm:text-3xl';
      default: return 'text-lg sm:text-xl';
    }
  }

  function getConfigFontSizeClass(offset) {
    switch (offset) {
      case -2: return 'text-[8px]';
      case -1: return 'text-[10px]';
      case 1:  return 'text-sm';
      case 2:  return 'text-base';
      default: return 'text-xs';
    }
  }
  
  // Copy-paste state
  let copiedButton = null;

  // Command testing state
  let commandTesting = false;
  let commandTestResult = '';
  let commandTestSuccess = true;

  // Touch screen feedback
  let activeClickId = null;

  onMount(async () => {
    await loadAppConfig();
    await loadExistingImages();
  });

  async function loadAppConfig() {
    try {
      const cfg = await LoadConfig();
      // Ensure structures are initialized
      config = {
        rows: cfg.rows || 2,
        cols: cfg.cols || 4,
        pages: cfg.pages || []
      };
      await loadAllImages();
    } catch (err) {
      console.error("Error loading config:", err);
    }
  }

  async function loadAllImages() {
    const cache = {};
    if (config.pages) {
      for (const page of config.pages) {
        if (page.buttons) {
          for (const btn of page.buttons) {
            if (btn.bgImage) {
              try {
                const b64 = await GetImageBase64(btn.bgImage);
                cache[btn.id] = b64;
              } catch (e) {
                console.error("Failed to load image for button:", btn.id, e);
              }
            }
          }
        }
      }
    }
    base64Cache = cache;
  }

  async function loadSingleImage(btnId, path) {
    if (!path) {
      base64Cache[btnId] = '';
      base64Cache = { ...base64Cache };
      return;
    }
    try {
      const b64 = await GetImageBase64(path);
      base64Cache[btnId] = b64;
      base64Cache = { ...base64Cache };
    } catch (e) {
      console.error("Failed to load single image:", e);
    }
  }

  // Get button configured for slot index on current page
  function getButtonAtSlot(index, pages, activePage) {
    if (!pages) return null;
    const page = pages.find(p => p.pageIndex === activePage);
    if (!page || !page.buttons) return null;
    return page.buttons.find(b => b.order === index);
  }

  // Handle button click in standard deck mode
  async function handleButtonClick(button) {
    if (!button || !button.command) return;
    
    // Provide tactile click animation
    activeClickId = button.id;
    setTimeout(() => {
      activeClickId = null;
    }, 150);

    try {
      await RunCommandAsync(button.command);
    } catch (err) {
      console.error("Failed to run command:", err);
    }
  }

  // Config View: select a grid slot to edit
  function selectSlot(index) {
    const lastSlot = (config.rows * config.cols) - 1;
    const prevLastSlot = lastSlot - 1;
    // Prevent configuring last two slots since they are reserved for pagination
    if (index === lastSlot || index === prevLastSlot) {
      selectedSlot = null;
      return;
    }
    selectedSlot = index;
    commandTestResult = '';
    
    const existing = getButtonAtSlot(index, config.pages, currentPage);
    if (existing) {
      editId = existing.id;
      editLabel = existing.label;
      editCommand = existing.command;
      editBgImage = existing.bgImage;
      editBgColor = existing.bgColor || '#1f2937';
      editFontColor = existing.fontColor || '#ffffff';
      editFontSize = existing.fontSize || 0;
    } else {
      // Initialize new button template
      editId = 'btn_' + Math.random().toString(36).substr(2, 9);
      editLabel = '';
      editCommand = '';
      editBgImage = '';
      editBgColor = '#1f2937';
      editFontColor = '#ffffff';
      editFontSize = 0;
    }
  }

  // Save the currently edited button
  async function saveButton() {
    if (selectedSlot === null) return;

    // Create updated button
    const updatedButton = {
      id: editId,
      label: editLabel,
      command: editCommand,
      bgImage: editBgImage,
      bgColor: editBgColor,
      fontColor: editFontColor,
      fontSize: editFontSize,
      order: selectedSlot
    };

    if (!config.pages) {
      config.pages = [];
    }

    let page = config.pages.find(p => p.pageIndex === currentPage);
    if (!page) {
      page = { pageIndex: currentPage, buttons: [] };
      config.pages.push(page);
    }

    page.buttons = page.buttons.filter(b => b.order !== selectedSlot);
    page.buttons.push(updatedButton);

    // Force reactivity update in Svelte
    config = { ...config };
    
    // Refresh base64 image cache
    if (editBgImage) {
      await loadSingleImage(editId, editBgImage);
    }

    await saveConfig();
  }

  // Clear/delete button from configuration
  async function deleteButton() {
    if (selectedSlot === null) return;
    
    if (config.pages) {
      let page = config.pages.find(p => p.pageIndex === currentPage);
      if (page && page.buttons) {
        page.buttons = page.buttons.filter(b => b.order !== selectedSlot);
      }
    }

    // Force reactivity update in Svelte
    config = { ...config };
    
    // Clear editing fields
    editLabel = '';
    editCommand = '';
    editBgImage = '';
    
    await saveConfig();
  }

  // Copy button template to clipboard
  function copyButton() {
    copiedButton = {
      label: editLabel,
      command: editCommand,
      bgImage: editBgImage,
      bgColor: editBgColor,
      fontColor: editFontColor,
      fontSize: editFontSize
    };
  }

  // Paste button template from clipboard
  async function pasteButton() {
    if (!copiedButton || selectedSlot === null) return;
    editLabel = copiedButton.label;
    editCommand = copiedButton.command;
    editBgImage = copiedButton.bgImage;
    editBgColor = copiedButton.bgColor || '#1f2937';
    editFontColor = copiedButton.fontColor || '#ffffff';
    editFontSize = copiedButton.fontSize || 0;
    await saveButton();
  }

  // Select image from file dialog
  async function handleSelectImage() {
    try {
      const selectedPath = await SelectImage();
      if (!selectedPath) return; // cancelled

      // Copy image to TouchDeck's config directory
      const copiedPath = await CopyImageToConfig(selectedPath);
      editBgImage = copiedPath;
      await loadExistingImages();
    } catch (err) {
      console.error("Error picking image:", err);
    }
  }

  // Clear background image from editor
  function clearImage() {
    editBgImage = '';
  }

  // Save configuration on backend
  async function saveConfig() {
    try {
      await SaveConfig(config);
      await loadAppConfig(); // reload & sync
    } catch (err) {
      console.error("Error saving config:", err);
    }
  }

  // Test button script synchronously
  async function testCommand() {
    if (!editCommand) {
      commandTestResult = "No command entered to test!";
      commandTestSuccess = false;
      return;
    }
    
    commandTesting = true;
    commandTestResult = "Running...";
    commandTestSuccess = true;

    try {
      const output = await RunCommandSync(editCommand);
      commandTestResult = output || "Command executed successfully with no output.";
      commandTestSuccess = true;
    } catch (err) {
      commandTestResult = `Error:\n${err}`;
      commandTestSuccess = false;
    } finally {
      commandTesting = false;
    }
  }

  // Move button to swap with another slot on the current page
  async function moveButton(direction) {
    if (selectedSlot === null) return;
    
    let targetSlot = selectedSlot;
    const rows = config.rows;
    const cols = config.cols;
    const lastSlot = (rows * cols) - 1;
    const prevLastSlot = lastSlot - 1;
    
    const row = Math.floor(selectedSlot / cols);
    const col = selectedSlot % cols;

    if (direction === 'left' && col > 0) targetSlot = selectedSlot - 1;
    else if (direction === 'right' && col < cols - 1) targetSlot = selectedSlot + 1;
    else if (direction === 'up' && row > 0) targetSlot = selectedSlot - cols;
    else if (direction === 'down' && row < rows - 1) targetSlot = selectedSlot + cols;

    // Prevent moving onto or swapping with page switcher slots
    if (targetSlot === selectedSlot || targetSlot === lastSlot || targetSlot === prevLastSlot) return;

    if (!config.pages) return;
    let page = config.pages.find(p => p.pageIndex === currentPage);
    if (!page || !page.buttons) return;

    const btnAtSelected = page.buttons.find(b => b.order === selectedSlot);
    const btnAtTarget = page.buttons.find(b => b.order === targetSlot);

    let updatedButtons = page.buttons.filter(b => b.order !== selectedSlot && b.order !== targetSlot);

    if (btnAtSelected) {
      updatedButtons.push({ ...btnAtSelected, order: targetSlot });
    }
    if (btnAtTarget) {
      updatedButtons.push({ ...btnAtTarget, order: selectedSlot });
    }

    page.buttons = updatedButtons;
    config = { ...config };
    selectedSlot = targetSlot; // follow selection to new slot
    await saveConfig();
  }

  // Modify Grid Size
  async function adjustGridSize(dimension, delta) {
    if (dimension === 'rows') {
      const newRows = Math.max(1, config.rows + delta);
      config.rows = newRows;
    } else if (dimension === 'cols') {
      const newCols = Math.max(1, config.cols + delta);
      config.cols = newCols;
    }
    // Prune out-of-bounds buttons on all pages
    const maxSlots = config.rows * config.cols;
    if (config.pages) {
      for (let page of config.pages) {
        if (page.buttons) {
          page.buttons = page.buttons.filter(b => b.order < maxSlots);
        }
      }
    }
    config = { ...config };
    
    if (selectedSlot !== null && selectedSlot >= maxSlots) {
      selectedSlot = null;
    }
    
    await saveConfig();
  }
</script>

<main class="h-screen w-screen bg-[#111827] text-white flex flex-col select-none overflow-hidden font-sans">
  <!-- Header Bar -->
  <header class="bg-[#1f2937] border-b border-[#374151] px-4 py-3 flex items-center justify-between shadow-md shrink-0">
    <div class="flex items-center gap-3">
      <span class="text-2xl">📱</span>
      <h1 class="text-xl font-bold tracking-wide text-white">TouchDeck</h1>
    </div>
    
    <div class="flex gap-2">
      <button 
        class="px-5 py-2 rounded-lg font-medium transition-all duration-150 flex items-center gap-2 active:scale-95 {activeTab === 'deck' ? 'bg-blue-600 text-white shadow-lg shadow-blue-500/30' : 'bg-[#374151] hover:bg-[#4b5563] text-gray-300'}"
        on:click={() => { activeTab = 'deck'; selectedSlot = null; }}
      >
        <span>🖥️</span> Deck View
      </button>
      <button 
        class="px-5 py-2 rounded-lg font-medium transition-all duration-150 flex items-center gap-2 active:scale-95 {activeTab === 'config' ? 'bg-blue-600 text-white shadow-lg shadow-blue-500/30' : 'bg-[#374151] hover:bg-[#4b5563] text-gray-300'}"
        on:click={() => { activeTab = 'config'; selectSlot(0); }}
      >
        <span>⚙️</span> Configuration
      </button>
    </div>
  </header>

  <!-- Content Workspace -->
  <div class="flex-grow flex overflow-hidden">
    {#if activeTab === 'deck'}
      <!-- DECK VIEW -->
      <div class="flex-grow p-4 flex items-center justify-center bg-[#0b0f19]">
        <div 
          class="grid gap-4 w-full h-full max-w-full max-h-full items-stretch"
          style="
            grid-template-rows: repeat({config.rows}, minmax(0, 1fr));
            grid-template-columns: repeat({config.cols}, minmax(0, 1fr));
          "
        >
          {#each Array(config.rows * config.cols) as _, i}
            {#if i === (config.rows * config.cols) - 2}
              <!-- Special Prev Page Navigation Button (Full Size) -->
              <button
                class="relative rounded-2xl overflow-hidden border border-[#4b5563] bg-[#1f2937] hover:bg-[#2d3748] active:bg-[#3b82f6]/80 shadow-lg flex flex-col items-center justify-center p-3 text-center transition-all duration-150 transform active:scale-95 select-none"
                on:click={() => currentPage = (currentPage - 1 + totalPages) % totalPages}
              >
                <span class="text-2xl sm:text-3xl font-black mb-1 text-blue-400 drop-shadow-[0_1px_2px_rgba(0,0,0,0.8)]">◀</span>
                <span class="text-xs sm:text-sm uppercase font-bold text-gray-300 tracking-wider">Prev Page</span>
              </button>
            {:else if i === (config.rows * config.cols) - 1}
              <!-- Special Next Page Navigation Button (Full Size) -->
              <button
                class="relative rounded-2xl overflow-hidden border border-[#4b5563] bg-[#1f2937] hover:bg-[#2d3748] active:bg-[#3b82f6]/80 shadow-lg flex flex-col items-center justify-center p-3 text-center transition-all duration-150 transform active:scale-95 select-none"
                on:click={() => currentPage = (currentPage + 1) % totalPages}
              >
                <span class="text-2xl sm:text-3xl font-black mb-1 text-blue-400 drop-shadow-[0_1px_2px_rgba(0,0,0,0.8)]">▶</span>
                <span class="text-xs sm:text-sm uppercase font-bold text-gray-300 tracking-wider mb-2">Next Page</span>
                <span class="absolute bottom-2 bg-[#111827] border border-gray-600 px-2 py-0.5 rounded-full text-[10px] font-bold text-gray-400">
                  {(currentPage + 1) + " / " + totalPages}
                </span>
              </button>
            {:else}
              {#each [getButtonAtSlot(i, config.pages, currentPage)] as button}
                {#if button && (button.label || button.bgImage || button.command)}
                  <button
                    class="relative rounded-2xl overflow-hidden border border-[#374151] shadow-lg flex flex-col items-center justify-center p-3 text-center transition-all duration-150 transform active:scale-95 select-none"
                    style="
                      background-color: {activeClickId === button.id ? '#3b82f6' : (button.bgColor || '#1f2937')};
                      color: {button.fontColor || '#ffffff'};
                      background-image: {base64Cache[button.id] ? `url(${base64Cache[button.id]})` : 'none'};
                      background-size: cover;
                      background-position: center;
                      background-repeat: no-repeat;
                    "
                    on:click={() => handleButtonClick(button)}
                  >
                    <!-- Semi-transparent overlay to ensure text contrast over background image -->
                    {#if base64Cache[button.id]}
                      <div class="absolute inset-0 bg-black/40 z-0"></div>
                    {/if}
                    
                    <span class="{getFontSizeClass(button.fontSize)} font-bold tracking-wide relative z-10 break-words w-full select-none text-shadow max-w-full pointer-events-none drop-shadow-[0_2px_4px_rgba(0,0,0,0.8)]">
                      {button.label || ''}
                    </span>
                  </button>
                {:else}
                  <!-- Empty Unconfigured Button Slot -->
                  <div class="rounded-2xl border border-dashed border-[#374151]/60 bg-[#111827]/40 flex items-center justify-center">
                    <span class="text-gray-600 text-sm">--</span>
                  </div>
                {/if}
              {/each}
            {/if}
          {/each}
        </div>
      </div>
    {:else}
      <!-- CONFIGURATION VIEW -->
      <!-- Left Hand Side: Grid Preview & Selection -->
      <div class="w-1/2 p-4 flex flex-col border-r border-[#374151] bg-[#0b0f19] overflow-y-auto">
        <div class="mb-4 flex items-center justify-between shrink-0">
          <h2 class="text-lg font-bold text-gray-300">Live Grid Preview</h2>
          <div class="flex items-center gap-4 bg-[#1f2937] px-3 py-1.5 rounded-lg border border-[#374151]">
            <div class="flex items-center gap-2">
              <span class="text-xs font-semibold text-gray-400">Rows:</span>
              <button class="bg-[#374151] active:bg-[#4b5563] w-6 h-6 flex items-center justify-center rounded text-sm text-white" on:click={() => adjustGridSize('rows', -1)}>-</button>
              <span class="w-4 text-center font-bold text-sm text-white">{config.rows}</span>
              <button class="bg-[#374151] active:bg-[#4b5563] w-6 h-6 flex items-center justify-center rounded text-sm text-white" on:click={() => adjustGridSize('rows', 1)}>+</button>
            </div>
            <div class="flex items-center gap-2 border-l border-[#374151] pl-4">
              <span class="text-xs font-semibold text-gray-400">Cols:</span>
              <button class="bg-[#374151] active:bg-[#4b5563] w-6 h-6 flex items-center justify-center rounded text-sm text-white" on:click={() => adjustGridSize('cols', -1)}>-</button>
              <span class="w-4 text-center font-bold text-sm text-white">{config.cols}</span>
              <button class="bg-[#374151] active:bg-[#4b5563] w-6 h-6 flex items-center justify-center rounded text-sm text-white" on:click={() => adjustGridSize('cols', 1)}>+</button>
            </div>
          </div>
        </div>

        <!-- Page Swapping Selector for Configuration -->
        <div class="mb-4 bg-[#1f2937] rounded-xl border border-[#374151] p-2 flex items-center justify-between shrink-0">
          <span class="text-xs font-semibold uppercase tracking-wider text-gray-400 pl-2">Current Editing Page:</span>
          <div class="flex gap-1.5">
            {#each Array(totalPages) as _, idx}
              <button
                class="px-3 py-1 rounded-md text-xs font-black transition-all active:scale-95 {currentPage === idx ? 'bg-blue-600 text-white shadow-md shadow-blue-500/20' : 'bg-[#2d3748] hover:bg-[#374151] text-gray-300'}"
                on:click={() => { currentPage = idx; selectedSlot = null; }}
              >
                Page {idx + 1}
              </button>
            {/each}
          </div>
        </div>

        <div 
          class="grid gap-2 w-full flex-grow items-stretch select-none animate-fade-in"
          style="
            grid-template-rows: repeat({config.rows}, minmax(0, 1fr));
            grid-template-columns: repeat({config.cols}, minmax(0, 1fr));
            min-height: 280px;
          "
        >
          {#each Array(config.rows * config.cols) as _, i}
            {#if i === (config.rows * config.cols) - 2}
              <!-- Special Preview for Prev Page Button -->
              <div class="relative rounded-xl overflow-hidden border-2 border-dashed border-[#4b5563] bg-[#1f2937]/50 flex items-center justify-center p-2 text-center select-none text-[#9ca3af]">
                <span class="text-xs font-bold uppercase tracking-wider text-blue-400 drop-shadow-[0_1px_2px_rgba(0,0,0,0.8)]">
                  [Prev Page]
                </span>
              </div>
            {:else if i === (config.rows * config.cols) - 1}
              <!-- Special Preview for Next Page Button -->
              <div class="relative rounded-xl overflow-hidden border-2 border-dashed border-[#4b5563] bg-[#1f2937]/50 flex items-center justify-center p-2 text-center select-none text-[#9ca3af]">
                <span class="text-xs font-bold uppercase tracking-wider text-blue-400 drop-shadow-[0_1px_2px_rgba(0,0,0,0.8)]">
                  [Next Page]
                </span>
              </div>
            {:else}
              {#each [getButtonAtSlot(i, config.pages, currentPage)] as button}
                <button
                  class="relative rounded-xl overflow-hidden border-2 flex flex-col items-center justify-center p-2 text-center transition-all duration-100 {selectedSlot === i ? 'border-blue-500' : button ? 'border-[#374151] hover:bg-[#374151]/50' : 'border-dashed border-gray-700/80 hover:bg-[#111827]'}"
                  style="
                    background-color: {selectedSlot === i ? '#1e293b' : (button ? (button.bgColor || '#1f2937') : 'rgba(17, 24, 39, 0.3)')};
                    color: {button ? (button.fontColor || '#ffffff') : '#9ca3af'};
                    background-image: {button && base64Cache[button.id] ? `url(${base64Cache[button.id]})` : 'none'};
                    background-size: cover;
                    background-position: center;
                  "
                  on:click={() => selectSlot(i)}
                >
                  {#if button && base64Cache[button.id]}
                    <div class="absolute inset-0 bg-black/40 z-0"></div>
                  {/if}
                  
                  <span class="{getConfigFontSizeClass(button ? button.fontSize : 0)} font-bold relative z-10 break-all pointer-events-none drop-shadow-[0_1px_2px_rgba(0,0,0,0.8)]" style="color: inherit">
                    {button ? (button.label || '(No Label)') : `+ Slot ${i+1}`}
                  </span>
                </button>
              {/each}
            {/if}
          {/each}
        </div>
      </div>

      <!-- Right Hand Side: Button Configuration Fields -->
      <div class="w-1/2 p-6 flex flex-col bg-[#111827] overflow-y-auto">
        {#if selectedSlot !== null}
          <div class="flex items-center justify-between mb-4 pb-2 border-b border-[#374151] shrink-0">
            <h2 class="text-lg font-bold text-blue-400">Editing Slot {selectedSlot + 1}</h2>
            
            <div class="flex items-center gap-1 bg-[#1f2937] p-1 rounded-lg border border-[#374151]">
              <button 
                class="w-8 h-8 flex items-center justify-center bg-[#374151] hover:bg-[#4b5563] active:bg-blue-600 rounded text-sm font-bold text-white" 
                title="Move Left" 
                on:click={() => moveButton('left')}
              >
                ←
              </button>
              <button 
                class="w-8 h-8 flex items-center justify-center bg-[#374151] hover:bg-[#4b5563] active:bg-blue-600 rounded text-sm font-bold text-white" 
                title="Move Up" 
                on:click={() => moveButton('up')}
              >
                ↑
              </button>
              <button 
                class="w-8 h-8 flex items-center justify-center bg-[#374151] hover:bg-[#4b5563] active:bg-blue-600 rounded text-sm font-bold text-white" 
                title="Move Down" 
                on:click={() => moveButton('down')}
              >
                ↓
              </button>
              <button 
                class="w-8 h-8 flex items-center justify-center bg-[#374151] hover:bg-[#4b5563] active:bg-blue-600 rounded text-sm font-bold text-white" 
                title="Move Right" 
                on:click={() => moveButton('right')}
              >
                →
              </button>
            </div>
          </div>

          <div class="space-y-4 flex-grow">
            <!-- Label -->
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5" for="label">Button Label</label>
              <input
                id="label"
                type="text"
                bind:value={editLabel}
                placeholder="E.g. Launch Firefox"
                class="w-full bg-[#1f2937] border border-[#374151] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
              />
            </div>

            <!-- Command -->
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5" for="command">Shell Command / Script</label>
              <textarea
                id="command"
                bind:value={editCommand}
                placeholder="E.g. firefox & or /home/conrad/scripts/start.sh"
                rows="3"
                class="w-full bg-[#1f2937] border border-[#374151] rounded-lg px-3 py-2 text-sm text-white font-mono focus:outline-none focus:border-blue-500 resize-none"
              ></textarea>
            </div>

            <!-- Background Image -->
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5" for="image">Background Image</label>
              
              <!-- Dropdown of existing images -->
              <div class="mb-2">
                <select
                  class="w-full bg-[#1f2937] border border-[#374151] rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-blue-500"
                  bind:value={editBgImage}
                >
                  <option value="">-- No Image / Select Existing --</option>
                  {#each existingImages as imgPath}
                    <option value={imgPath}>{getFriendlyImageName(imgPath)}</option>
                  {/each}
                </select>
              </div>

              <div class="flex gap-2">
                <input
                  id="image"
                  type="text"
                  value={editBgImage ? getFriendlyImageName(editBgImage) : ""}
                  disabled
                  placeholder="No background image chosen"
                  class="flex-grow bg-[#1f2937]/30 border border-[#374151] rounded-lg px-3 py-2 text-xs text-gray-400 select-all font-mono"
                />
                <button
                  class="px-3 bg-blue-600 hover:bg-blue-500 active:scale-95 text-white rounded-lg text-xs font-bold transition-all whitespace-nowrap"
                  on:click={handleSelectImage}
                >
                  Choose File
                </button>
                {#if editBgImage}
                  <button
                    class="px-2 bg-red-600 hover:bg-red-500 active:scale-95 text-white rounded-lg text-xs font-bold transition-all"
                    title="Remove Image"
                    on:click={clearImage}
                  >
                    ✕
                  </button>
                {/if}
              </div>
            </div>

            <!-- Custom Tile Colors -->
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5">Tile Color</label>
                <div class="flex gap-2 items-center bg-[#1f2937] border border-[#374151] rounded-lg px-3 py-1.5">
                  <input
                    type="color"
                    bind:value={editBgColor}
                    class="w-8 h-8 rounded border border-gray-600 bg-transparent cursor-pointer"
                  />
                  <input
                    type="text"
                    bind:value={editBgColor}
                    class="w-full bg-transparent text-sm text-white font-mono focus:outline-none uppercase"
                    maxLength="7"
                  />
                </div>
              </div>
              <div>
                <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5">Text Color</label>
                <div class="flex gap-2 items-center bg-[#1f2937] border border-[#374151] rounded-lg px-3 py-1.5">
                  <input
                    type="color"
                    bind:value={editFontColor}
                    class="w-8 h-8 rounded border border-gray-600 bg-transparent cursor-pointer"
                  />
                  <input
                    type="text"
                    bind:value={editFontColor}
                    class="w-full bg-transparent text-sm text-white font-mono focus:outline-none uppercase"
                    maxLength="7"
                  />
                </div>
              </div>
            </div>

            <!-- Custom Font Size Offset -->
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5">Font Size Modifier</label>
              <div class="flex items-center bg-[#1f2937] border border-[#374151] rounded-lg p-1 gap-1">
                <button
                  type="button"
                  class="flex-1 py-1 text-[10px] font-bold rounded transition-all {editFontSize === -2 ? 'bg-blue-600 text-white shadow' : 'text-gray-400 hover:bg-[#2d3748]'}"
                  on:click={() => editFontSize = -2}
                >
                  -2 (Tiny)
                </button>
                <button
                  type="button"
                  class="flex-1 py-1 text-[10px] font-bold rounded transition-all {editFontSize === -1 ? 'bg-blue-600 text-white shadow' : 'text-gray-400 hover:bg-[#2d3748]'}"
                  on:click={() => editFontSize = -1}
                >
                  -1
                </button>
                <button
                  type="button"
                  class="flex-1 py-1 text-[10px] font-bold rounded transition-all {editFontSize === 0 ? 'bg-blue-600 text-white shadow' : 'text-gray-400 hover:bg-[#2d3748]'}"
                  on:click={() => editFontSize = 0}
                >
                  Default
                </button>
                <button
                  type="button"
                  class="flex-1 py-1 text-[10px] font-bold rounded transition-all {editFontSize === 1 ? 'bg-blue-600 text-white shadow' : 'text-gray-400 hover:bg-[#2d3748]'}"
                  on:click={() => editFontSize = 1}
                >
                  +1
                </button>
                <button
                  type="button"
                  class="flex-1 py-1 text-[10px] font-bold rounded transition-all {editFontSize === 2 ? 'bg-blue-600 text-white shadow' : 'text-gray-400 hover:bg-[#2d3748]'}"
                  on:click={() => editFontSize = 2}
                >
                  +2 (Huge)
                </button>
              </div>
            </div>

            <!-- Quick Template Copier / Clipboard Actions -->
            <div class="bg-[#1f2937] p-3 rounded-lg border border-[#374151] flex items-center justify-between shrink-0">
              <span class="text-xs font-medium text-gray-400">Button Templates:</span>
              <div class="flex gap-2">
                <button
                  class="px-3 py-1 bg-[#374151] hover:bg-[#4b5563] text-gray-200 rounded-md text-xs font-bold active:scale-95 transition-all"
                  on:click={copyButton}
                >
                  📋 Copy Settings
                </button>
                <button
                  class="px-3 py-1 bg-[#374151] hover:bg-[#4b5563] text-gray-200 rounded-md text-xs font-bold active:scale-95 transition-all disabled:opacity-40 disabled:cursor-not-allowed"
                  disabled={!copiedButton}
                  on:click={pasteButton}
                >
                  📥 Paste Settings
                </button>
              </div>
            </div>

            <!-- Actions Panel -->
            <div class="flex gap-3 pt-3">
              <button
                class="flex-1 py-2.5 bg-green-600 hover:bg-green-500 active:scale-95 text-white rounded-lg font-bold text-sm tracking-wide transition-all shadow-md shadow-green-700/20"
                on:click={saveButton}
              >
                💾 Save Changes
              </button>
              <button
                class="px-4 py-2.5 bg-red-600 hover:bg-red-500 active:scale-95 text-white rounded-lg font-bold text-sm transition-all"
                title="Delete Button Config"
                on:click={deleteButton}
              >
                🗑️ Clear Slot
              </button>
            </div>

            <!-- Command execution sandbox test -->
            <div class="mt-6 pt-4 border-t border-[#374151]">
              <div class="flex items-center justify-between mb-2">
                <span class="text-xs font-semibold uppercase tracking-wider text-gray-400">Test Running Script</span>
                <button
                  class="px-3 py-1 bg-yellow-600 hover:bg-yellow-500 active:scale-95 text-white rounded-md text-xs font-bold transition-all disabled:opacity-50 text-white"
                  on:click={testCommand}
                  disabled={commandTesting}
                >
                  {commandTesting ? 'Executing...' : '⚡ Test Run Now'}
                </button>
              </div>
              <div class="bg-[#0b0f19] border border-[#374151] rounded-lg p-3 h-28 overflow-y-auto font-mono text-xs">
                {#if commandTestResult}
                  <pre class={commandTestSuccess ? 'text-green-400' : 'text-red-400'}>{commandTestResult}</pre>
                {:else}
                  <span class="text-gray-500 italic">No execution logs. Click Test Run to execute this command in Linux bash.</span>
                {/if}
              </div>
            </div>
          </div>
        {:else}
          <div class="h-full flex flex-col items-center justify-center text-center text-gray-500">
            <span class="text-4xl mb-2">👈</span>
            <p class="text-sm font-semibold">Select any grid slot on the left to start editing, copying, or arranging buttons.</p>
          </div>
        {/if}
      </div>
    {/if}
  </div>
</main>

<style>
  /* Enable text outline shadow for button labels overlaying image backgrounds */
  .text-shadow {
    text-shadow: 2px 2px 4px rgba(0, 0, 0, 1), -2px -2px 4px rgba(0, 0, 0, 1), 2px -2px 4px rgba(0, 0, 0, 1), -2px 2px 4px rgba(0, 0, 0, 1);
  }
</style>
