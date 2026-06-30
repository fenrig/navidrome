import React, { useCallback, useState } from 'react'
import { useDispatch } from 'react-redux'
import { useGetOne } from 'react-admin'
import { GlobalHotKeys } from 'react-hotkeys'
import IconButton from '@material-ui/core/IconButton'
import Tooltip from '@material-ui/core/Tooltip'
import { useMediaQuery } from '@material-ui/core'
import { RiSaveLine } from 'react-icons/ri'
import { LoveButton, useToggleLove } from '../common'
import { addTracks, openSaveQueueDialog } from '../actions'
import { keyMap } from '../hotkeys'
import { makeStyles } from '@material-ui/core/styles'
import { httpClient } from '../dataProvider'
import { REST_URL } from '../consts'
import { useNotify, useTranslate } from 'react-admin'
import QueueMusicIcon from '@material-ui/icons/QueueMusic'

const useStyles = makeStyles((theme) => ({
  toolbar: {
    display: 'flex',
    alignItems: 'center',
    flexGrow: 1,
    justifyContent: 'flex-end',
    gap: '0.5rem',
    listStyle: 'none',
    padding: 0,
    margin: 0,
  },
  mobileListItem: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    listStyle: 'none',
    padding: theme.spacing(0.5),
    margin: 0,
    height: 24,
  },
  button: {
    width: '2.5rem',
    height: '2.5rem',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: 0,
  },
  mobileButton: {
    width: 24,
    height: 24,
    padding: 0,
    margin: 0,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '18px',
  },
  mobileIcon: {
    fontSize: '18px',
    display: 'flex',
    alignItems: 'center',
  },
}))

const PlayerToolbar = ({ id, isRadio }) => {
  const dispatch = useDispatch()
  const translate = useTranslate()
  const notify = useNotify()
  const { data, loading } = useGetOne('song', id, { enabled: !!id && !isRadio })
  const [toggleLove, toggling] = useToggleLove('song', data)
  const isDesktop = useMediaQuery('(min-width:810px)')
  const classes = useStyles()
  const [autofilling, setAutofilling] = useState(false)

  const handlers = {
    TOGGLE_LOVE: useCallback(() => toggleLove(), [toggleLove]),
  }

  const handleSaveQueue = useCallback(
    (e) => {
      dispatch(openSaveQueueDialog())
      e.stopPropagation()
    },
    [dispatch],
  )

  const handleAutofillQueue = useCallback(
    async (e) => {
      e.stopPropagation()
      setAutofilling(true)
      try {
        const response = await httpClient(`${REST_URL}/queue/autofill?count=1`, {
          method: 'POST',
        })
        const tracks = response.json || []
        if (tracks.length > 0) {
          const data = tracks.reduce((acc, track) => {
            acc[track.id] = track
            return acc
          }, {})
          dispatch(addTracks(data, tracks.map((track) => track.id)))
        }
      } catch {
        notify('ra.page.error', { type: 'warning' })
      } finally {
        setAutofilling(false)
      }
    },
    [dispatch, notify],
  )

  const buttonClass = isDesktop ? classes.button : classes.mobileButton
  const listItemClass = isDesktop ? classes.toolbar : classes.mobileListItem

  const saveQueueButton = (
    <IconButton
      size={isDesktop ? 'small' : undefined}
      onClick={handleSaveQueue}
      disabled={isRadio}
      data-testid="save-queue-button"
      className={buttonClass}
    >
      <RiSaveLine className={!isDesktop ? classes.mobileIcon : undefined} />
    </IconButton>
  )

  const autofillQueueButton = (
    <Tooltip
      title={translate('resources.playlist.actions.autofillQueue', {
        _: 'Autofill queue',
      })}
    >
      <span>
        <IconButton
          size={isDesktop ? 'small' : undefined}
          onClick={handleAutofillQueue}
          disabled={isRadio || autofilling}
          data-testid="autofill-queue-button"
          className={buttonClass}
        >
          <QueueMusicIcon className={!isDesktop ? classes.mobileIcon : undefined} />
        </IconButton>
      </span>
    </Tooltip>
  )

  const loveButton = (
    <LoveButton
      record={data}
      resource={'song'}
      size={isDesktop ? undefined : 'inherit'}
      disabled={loading || toggling || !id || isRadio}
      className={buttonClass}
    />
  )

  return (
    <>
      <GlobalHotKeys keyMap={keyMap} handlers={handlers} allowChanges />
      {isDesktop ? (
        <li className={`${listItemClass} item`}>
          {saveQueueButton}
          {autofillQueueButton}
          {loveButton}
        </li>
      ) : (
        <>
          <li className={`${listItemClass} item`}>{saveQueueButton}</li>
          <li className={`${listItemClass} item`}>{autofillQueueButton}</li>
          <li className={`${listItemClass} item`}>{loveButton}</li>
        </>
      )}
    </>
  )
}

export default PlayerToolbar
