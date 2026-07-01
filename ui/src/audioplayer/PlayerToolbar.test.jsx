import React from 'react'
import {
  render,
  screen,
  fireEvent,
  cleanup,
  waitFor,
} from '@testing-library/react'
import { useMediaQuery } from '@material-ui/core'
import { useGetOne } from 'react-admin'
import { useDispatch, useSelector } from 'react-redux'
import { useToggleLove } from '../common'
import { addTracks, openSaveQueueDialog, toggleQueueAutofill } from '../actions'
import PlayerToolbar from './PlayerToolbar'
import { httpClient } from '../dataProvider'

// Mock dependencies
vi.mock('@material-ui/core', async () => {
  const actual = await import('@material-ui/core')
  return {
    ...actual,
    useMediaQuery: vi.fn(),
  }
})

vi.mock('react-admin', () => ({
  useGetOne: vi.fn(),
  useNotify: () => vi.fn(),
  useTranslate: () => (key, options) => options?._ || key,
}))

vi.mock('react-redux', () => ({
  useDispatch: vi.fn(),
  useSelector: vi.fn(),
}))

vi.mock('../common', () => ({
  LoveButton: ({ className, disabled }) => (
    <button data-testid="love-button" className={className} disabled={disabled}>
      Love
    </button>
  ),
  useToggleLove: vi.fn(),
}))

vi.mock('../actions', () => ({
  addTracks: vi.fn((data, ids) => ({ type: 'PLAYER_ADD_TRACKS', data, ids })),
  toggleQueueAutofill: vi.fn(() => ({ type: 'PLAYER_TOGGLE_QUEUE_AUTOFILL' })),
  openSaveQueueDialog: vi.fn(),
}))

vi.mock('../dataProvider', () => ({
  httpClient: vi.fn(),
}))

vi.mock('react-hotkeys', () => ({
  GlobalHotKeys: () => <div data-testid="global-hotkeys" />,
}))

describe('<PlayerToolbar />', () => {
  const mockToggleLove = vi.fn()
  const mockDispatch = vi.fn()
  const mockSongData = { id: 'song-1', name: 'Test Song', starred: false }

  beforeEach(() => {
    vi.clearAllMocks()
    useGetOne.mockReturnValue({ data: mockSongData, loading: false })
    useToggleLove.mockReturnValue([mockToggleLove, false])
    useDispatch.mockReturnValue(mockDispatch)
    useSelector.mockImplementation((selector) =>
      selector({
        player: { autofillEnabled: false, seenTrackIds: ['seed-1'] },
      }),
    )
    openSaveQueueDialog.mockReturnValue({ type: 'OPEN_SAVE_QUEUE_DIALOG' })
    toggleQueueAutofill.mockReturnValue({
      type: 'PLAYER_TOGGLE_QUEUE_AUTOFILL',
    })
    httpClient.mockResolvedValue({ json: [{ id: 'r1', title: 'Recommended' }] })
  })

  afterEach(cleanup)

  describe('Desktop layout', () => {
    beforeEach(() => {
      useMediaQuery.mockReturnValue(true) // isDesktop = true
    })

    it('renders desktop toolbar with both buttons', () => {
      // Actually three controls: save, autofill once, and dynamic toggle.
      render(<PlayerToolbar id="song-1" />)

      // Both buttons should be in a single list item
      const listItems = screen.getAllByRole('listitem')
      expect(listItems).toHaveLength(1)

      // Verify both buttons are rendered
      expect(screen.getByTestId('save-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('autofill-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('dynamic-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('love-button')).toBeInTheDocument()

      // Verify desktop classes are applied
      expect(listItems[0].className).toContain('toolbar')
    })

    it('disables save queue button when isRadio is true', () => {
      render(<PlayerToolbar id="song-1" isRadio={true} />)

      const saveQueueButton = screen.getByTestId('save-queue-button')
      expect(saveQueueButton).toBeDisabled()
    })

    it('disables love button when conditions are met', () => {
      useGetOne.mockReturnValue({ data: mockSongData, loading: true })

      render(<PlayerToolbar id="song-1" />)

      const loveButton = screen.getByTestId('love-button')
      expect(loveButton).toBeDisabled()
    })

    it('opens save queue dialog when save button is clicked', () => {
      render(<PlayerToolbar id="song-1" />)

      const saveQueueButton = screen.getByTestId('save-queue-button')
      fireEvent.click(saveQueueButton)

      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'OPEN_SAVE_QUEUE_DIALOG',
      })
    })

    it('toggles dynamic queue mode when toggle button is clicked', () => {
      render(<PlayerToolbar id="song-1" />)

      fireEvent.click(screen.getByTestId('dynamic-queue-button'))

      expect(toggleQueueAutofill).toHaveBeenCalled()
      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'PLAYER_TOGGLE_QUEUE_AUTOFILL',
      })
    })

    it('autofills the queue when autofill button is clicked', async () => {
      render(<PlayerToolbar id="song-1" />)

      fireEvent.click(screen.getByTestId('autofill-queue-button'))

      await waitFor(() => {
        expect(httpClient).toHaveBeenCalledWith(
          '/api/queue/autofill?count=1',
          expect.objectContaining({
            method: 'POST',
            body: JSON.stringify({
              count: 1,
              excludeIds: ['seed-1'],
            }),
          }),
        )
      })

      expect(addTracks).toHaveBeenCalledWith(
        { r1: { id: 'r1', title: 'Recommended' } },
        ['r1'],
      )
      expect(mockDispatch).toHaveBeenCalledWith({
        type: 'PLAYER_ADD_TRACKS',
        data: { r1: { id: 'r1', title: 'Recommended' } },
        ids: ['r1'],
      })
    })
  })

  describe('Mobile layout', () => {
    beforeEach(() => {
      useMediaQuery.mockReturnValue(false) // isDesktop = false
    })

    it('renders mobile toolbar with buttons in separate list items', () => {
      render(<PlayerToolbar id="song-1" />)

      // Each button should be in its own list item
      const listItems = screen.getAllByRole('listitem')
      expect(listItems).toHaveLength(4)

      // Verify both buttons are rendered
      expect(screen.getByTestId('save-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('autofill-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('dynamic-queue-button')).toBeInTheDocument()
      expect(screen.getByTestId('love-button')).toBeInTheDocument()

      // Verify mobile classes are applied
      expect(listItems[0].className).toContain('mobileListItem')
      expect(listItems[1].className).toContain('mobileListItem')
    })

    it('disables save queue button when isRadio is true', () => {
      render(<PlayerToolbar id="song-1" isRadio={true} />)

      const saveQueueButton = screen.getByTestId('save-queue-button')
      expect(saveQueueButton).toBeDisabled()
    })

    it('disables love button when conditions are met', () => {
      useGetOne.mockReturnValue({ data: mockSongData, loading: true })

      render(<PlayerToolbar id="song-1" />)

      const loveButton = screen.getByTestId('love-button')
      expect(loveButton).toBeDisabled()
    })
  })

  describe('Common behavior', () => {
    it('renders global hotkeys in both layouts', () => {
      // Test desktop layout
      useMediaQuery.mockReturnValue(true)
      render(<PlayerToolbar id="song-1" />)
      expect(screen.getByTestId('global-hotkeys')).toBeInTheDocument()

      // Cleanup and test mobile layout
      cleanup()
      useMediaQuery.mockReturnValue(false)
      render(<PlayerToolbar id="song-1" />)
      expect(screen.getByTestId('global-hotkeys')).toBeInTheDocument()
    })

    it('disables buttons when id is not provided', () => {
      render(<PlayerToolbar />)

      const loveButton = screen.getByTestId('love-button')
      expect(loveButton).toBeDisabled()
    })
  })
})
