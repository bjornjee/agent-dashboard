package tui

import "testing"

func TestPanelHeights(t *testing.T) {
	tests := []struct {
		name        string
		panelHeight int
		headerLines int
		wantFiles   int
		wantHistory int
		wantMsg     int
	}{
		{
			name:        "large terminal, parent agent header (10 lines)",
			panelHeight: 50,
			headerLines: 10,
			wantFiles:   5,  // 35 available * 15% = 5
			wantHistory: 10, // 35 available * 30% = 10
			wantMsg:     20, // 35 - 5 - 10 = 20
		},
		{
			name:        "medium terminal, parent agent header",
			panelHeight: 35,
			headerLines: 10,
			wantFiles:   3,  // 20 available * 15% = 3
			wantHistory: 6,  // 20 available * 30% = 6
			wantMsg:     11, // 20 - 3 - 6 = 11
		},
		{
			name:        "large terminal, subagent header (4 lines)",
			panelHeight: 50,
			headerLines: 4,
			wantFiles:   6,  // 41 available * 15% = 6
			wantHistory: 12, // 41 available * 30% = 12
			wantMsg:     23, // 41 - 6 - 12 = 23
		},
		{
			name:        "small terminal – hits minimums",
			panelHeight: 25,
			headerLines: 10,
			wantFiles:   3, // min
			wantHistory: 5, // min
			wantMsg:     5, // available < sum of minimums → early return
		},
		{
			name:        "tiny terminal – all minimums",
			panelHeight: 20,
			headerLines: 10,
			wantFiles:   3,
			wantHistory: 5,
			wantMsg:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filesH, historyH, msgH := panelHeights(tt.panelHeight, tt.headerLines)
			if filesH != tt.wantFiles {
				t.Errorf("filesH = %d, want %d", filesH, tt.wantFiles)
			}
			if historyH != tt.wantHistory {
				t.Errorf("historyH = %d, want %d", historyH, tt.wantHistory)
			}
			if msgH != tt.wantMsg {
				t.Errorf("msgH = %d, want %d", msgH, tt.wantMsg)
			}
		})
	}
}

func TestPanelHeights_LiveGetsLargestShare(t *testing.T) {
	// For any reasonable terminal size, Live > History > Files
	for panelHeight := 30; panelHeight <= 80; panelHeight++ {
		filesH, historyH, msgH := panelHeights(panelHeight, 10)
		if msgH <= historyH {
			t.Errorf("panelHeight=%d: msgH(%d) should be > historyH(%d)", panelHeight, msgH, historyH)
		}
		if historyH <= filesH {
			t.Errorf("panelHeight=%d: historyH(%d) should be > filesH(%d)", panelHeight, historyH, filesH)
		}
	}
}

func TestPanelHeights_SumsToAvailable(t *testing.T) {
	for panelHeight := 25; panelHeight <= 80; panelHeight++ {
		for _, headerLines := range []int{4, 8, 10, 12} {
			filesH, historyH, msgH := panelHeights(panelHeight, headerLines)
			available := panelHeight - headerLines - sectionGaps
			got := filesH + historyH + msgH
			if got != available && available >= minFilesHeight+minHistoryHeight+minMessageHeight {
				t.Errorf("panelHeight=%d headerLines=%d: sum(%d) != available(%d)",
					panelHeight, headerLines, got, available)
			}
		}
	}
}

func TestPanelHeights_DynamicHeaderAffectsViewports(t *testing.T) {
	// With a larger header, viewports should get less space
	_, _, msgSmallHeader := panelHeights(50, 4)
	_, _, msgLargeHeader := panelHeights(50, 10)
	if msgSmallHeader <= msgLargeHeader {
		t.Errorf("small header msgH(%d) should be > large header msgH(%d)",
			msgSmallHeader, msgLargeHeader)
	}
}
