package main

import "testing"

func TestPanelHeights(t *testing.T) {
	tests := []struct {
		name        string
		panelHeight int
		wantFiles   int
		wantHistory int
		wantMsg     int
	}{
		{
			name:        "large terminal (50 lines panel)",
			panelHeight: 50,
			wantFiles:   5,  // 36 available * 15% = 5
			wantHistory: 10, // 36 available * 30% = 10
			wantMsg:     21, // 36 - 5 - 10 = 21
		},
		{
			name:        "medium terminal (35 lines panel)",
			panelHeight: 35,
			wantFiles:   3,  // 21 available * 15% = 3
			wantHistory: 6,  // 21 available * 30% = 6
			wantMsg:     12, // 21 - 3 - 6 = 12
		},
		{
			name:        "small terminal – hits minimums",
			panelHeight: 25,
			wantFiles:   3, // min
			wantHistory: 5, // min
			wantMsg:     5, // available < sum of minimums → early return
		},
		{
			name:        "tiny terminal – all minimums",
			panelHeight: 20,
			wantFiles:   3,
			wantHistory: 5,
			wantMsg:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filesH, historyH, msgH := panelHeights(tt.panelHeight)
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
		filesH, historyH, msgH := panelHeights(panelHeight)
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
		filesH, historyH, msgH := panelHeights(panelHeight)
		available := panelHeight - headerLines - sectionGaps
		got := filesH + historyH + msgH
		if got != available && available >= minFilesHeight+minHistoryHeight+minMessageHeight {
			t.Errorf("panelHeight=%d: sum(%d) != available(%d)", panelHeight, got, available)
		}
	}
}
