import React from 'react';
import PropTypes from 'prop-types';
import { Link, withRouter } from 'react-router-dom';
import { withTranslation } from 'react-i18next';
import uuid from 'uuidv4';
import { send } from '@giantmachines/redux-websocket';
import { connect } from 'react-redux';
import { interval } from 'rxjs';
import {
  Breadcrumb,
  KPI,
  ImageCard,
  Table,
  TableHeader,
  TableBody,
  TableRow,
  Icon,
  Ellipse,
  Button,
  Card,
  SetupBox,
  Modal,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from '@kerberos-io/ui';
import './Dashboard.scss';
import ReactTooltip from 'react-tooltip';
import config from '../../config';
import { getConfig } from '../../actions/agent';

// eslint-disable-next-line react/prefer-stateless-function
class Dashboard extends React.Component {
  constructor() {
    super();
    this.state = {
      liveviewLoaded: false,
      liveviewMode: 'webrtc',
      open: false,
      currentRecording: '',
      initialised: false,
    };
    this.videoRef = React.createRef();
    this.pendingRemoteCandidates = [];
    this.initialiseLiveview = this.initialiseLiveview.bind(this);
    this.initialiseSDLiveview = this.initialiseSDLiveview.bind(this);
    this.startWebRTCLiveview = this.startWebRTCLiveview.bind(this);
    this.handleWebRTCSignalMessage = this.handleWebRTCSignalMessage.bind(this);
    this.fallbackToSDLiveview = this.fallbackToSDLiveview.bind(this);
  }

  componentDidMount() {
    const { dispatchGetConfig } = this.props;
    dispatchGetConfig(() => this.initialiseLiveview());
    this.initialiseLiveview();
  }

  componentDidUpdate(prevProps) {
    const { images, dashboard } = this.props;
    const { liveviewLoaded, liveviewMode } = this.state;
    const configLoaded = this.hasAgentConfig(this.props);
    const prevConfigLoaded = this.hasAgentConfig(prevProps);

    if (!prevConfigLoaded && configLoaded) {
      this.initialiseLiveview();
    }

    if (
      liveviewMode === 'sd' &&
      !liveviewLoaded &&
      prevProps.images !== images &&
      images.length > 0
    ) {
      this.setState({
        liveviewLoaded: true,
      });
    }

    if (!prevProps.dashboard.cameraOnline && dashboard.cameraOnline) {
      this.initialiseLiveview();
    }
  }

  componentWillUnmount() {
    this.stopSDLiveview();
    this.stopWebRTCLiveview();
  }

  handleClose() {
    this.setState({
      open: false,
      currentRecording: '',
    });
  }

  // eslint-disable-next-line react/sort-comp
  hasAgentConfig(props) {
    const currentProps = props || this.props;
    const { config: configResponse } = currentProps;
    return !!(configResponse && configResponse.config);
  }

  browserSupportsWebRTC() {
    return (
      typeof window !== 'undefined' &&
      typeof window.RTCPeerConnection !== 'undefined'
    );
  }

  buildPeerConnectionConfig() {
    const { config: configResponse } = this.props;
    const agentConfig =
      configResponse && configResponse.config ? configResponse.config : {};
    const iceServers = [];

    if (agentConfig.stunuri) {
      iceServers.push({
        urls: [agentConfig.stunuri],
      });
    }

    if (agentConfig.turnuri) {
      const turnServer = {
        urls: [agentConfig.turnuri],
      };

      if (agentConfig.turn_username) {
        turnServer.username = agentConfig.turn_username;
      }

      if (agentConfig.turn_password) {
        turnServer.credential = agentConfig.turn_password;
      }

      iceServers.push(turnServer);
    }

    return {
      iceServers,
      iceTransportPolicy: agentConfig.turn_force === 'true' ? 'relay' : 'all',
    };
  }

  initialiseLiveview() {
    const { initialised } = this.state;
    const { dashboard } = this.props;

    if (initialised || !dashboard.cameraOnline) {
      return;
    }

    if (!this.hasAgentConfig()) {
      return;
    }

    if (this.browserSupportsWebRTC()) {
      this.startWebRTCLiveview();
    } else {
      this.fallbackToSDLiveview('WebRTC is not supported in this browser.');
    }
  }

  initialiseSDLiveview() {
    if (this.requestStreamSubscription) {
      return;
    }

    const message = {
      message_type: 'stream-sd',
    };
    const { connected, dispatchSend } = this.props;
    if (connected) {
      dispatchSend(message);
    }

    const requestStreamInterval = interval(2000);
    this.requestStreamSubscription = requestStreamInterval.subscribe(() => {
      const { connected: isConnected } = this.props;
      if (isConnected) {
        dispatchSend(message);
      }
    });
  }

  stopSDLiveview() {
    if (this.requestStreamSubscription) {
      this.requestStreamSubscription.unsubscribe();
      this.requestStreamSubscription = null;
    }

    const { dispatchSend } = this.props;
    dispatchSend({
      message_type: 'stop-sd',
    });
  }

  stopWebRTCLiveview() {
    if (this.webrtcTimeout) {
      window.clearTimeout(this.webrtcTimeout);
      this.webrtcTimeout = null;
    }

    if (this.webrtcSocket) {
      this.webrtcSocket.onopen = null;
      this.webrtcSocket.onmessage = null;
      this.webrtcSocket.onerror = null;
      this.webrtcSocket.onclose = null;
      this.webrtcSocket.close();
      this.webrtcSocket = null;
    }

    if (this.webrtcPeerConnection) {
      this.webrtcPeerConnection.ontrack = null;
      this.webrtcPeerConnection.onicecandidate = null;
      this.webrtcPeerConnection.onconnectionstatechange = null;
      this.webrtcPeerConnection.close();
      this.webrtcPeerConnection = null;
    }

    this.pendingRemoteCandidates = [];
    this.webrtcOfferStarted = false;
    this.webrtcSessionId = null;
    this.webrtcClientId = null;

    if (this.videoRef.current) {
      this.videoRef.current.srcObject = null;
    }
  }

  sendWebRTCMessage(messageType, message = {}) {
    if (!this.webrtcSocket || this.webrtcSocket.readyState !== WebSocket.OPEN) {
      return;
    }

    this.webrtcSocket.send(
      JSON.stringify({
        client_id: this.webrtcClientId,
        message_type: messageType,
        message,
      })
    );
  }

  async handleWebRTCSignalMessage(event) {
    let data;
    try {
      data = JSON.parse(event.data);
    } catch (error) {
      return;
    }

    const { message_type: messageType, message = {} } = data;
    const { session_id: sessionID, sdp, candidate } = message;

    if (messageType === 'hello-back') {
      await this.beginWebRTCLiveview();
      return;
    }

    if (sessionID && sessionID !== this.webrtcSessionId) {
      return;
    }

    switch (messageType) {
      case 'webrtc-answer':
        try {
          await this.webrtcPeerConnection.setRemoteDescription({
            type: 'answer',
            sdp: window.atob(sdp),
          });
          await this.flushPendingRemoteCandidates();
        } catch (error) {
          this.fallbackToSDLiveview(
            `Unable to apply WebRTC answer: ${error.message}`
          );
        }
        break;

      case 'webrtc-candidate': {
        try {
          const candidateInit = JSON.parse(candidate);
          if (
            this.webrtcPeerConnection.remoteDescription &&
            this.webrtcPeerConnection.remoteDescription.type
          ) {
            await this.webrtcPeerConnection.addIceCandidate(candidateInit);
          } else {
            this.pendingRemoteCandidates.push(candidateInit);
          }
        } catch (error) {
          this.fallbackToSDLiveview(
            `Unable to apply WebRTC candidate: ${error.message}`
          );
        }
        break;
      }

      case 'webrtc-error':
        this.fallbackToSDLiveview(
          message.message || 'The agent could not start the WebRTC liveview.'
        );
        break;

      default:
        break;
    }
  }

  async beginWebRTCLiveview() {
    if (!this.webrtcPeerConnection || this.webrtcOfferStarted) {
      return;
    }

    try {
      this.webrtcOfferStarted = true;
      const offer = await this.webrtcPeerConnection.createOffer({
        offerToReceiveAudio: true,
        offerToReceiveVideo: true,
      });
      await this.webrtcPeerConnection.setLocalDescription(offer);
      this.sendWebRTCMessage('stream-hd', {
        session_id: this.webrtcSessionId,
        sdp: window.btoa(this.webrtcPeerConnection.localDescription.sdp),
      });
    } catch (error) {
      this.fallbackToSDLiveview(
        `Unable to initialise WebRTC liveview: ${error.message}`
      );
    }
  }

  async flushPendingRemoteCandidates() {
    if (
      !this.webrtcPeerConnection ||
      !this.webrtcPeerConnection.remoteDescription
    ) {
      return;
    }

    while (this.pendingRemoteCandidates.length > 0) {
      const candidateInit = this.pendingRemoteCandidates.shift();
      try {
        // eslint-disable-next-line no-await-in-loop
        await this.webrtcPeerConnection.addIceCandidate(candidateInit);
      } catch (error) {
        this.fallbackToSDLiveview(
          `Unable to add remote ICE candidate: ${error.message}`
        );
        return;
      }
    }
  }

  startWebRTCLiveview() {
    if (this.webrtcPeerConnection || this.webrtcSocket) {
      return;
    }

    this.stopSDLiveview();

    this.webrtcClientId = uuid();
    this.webrtcSessionId = uuid();
    this.pendingRemoteCandidates = [];

    this.webrtcPeerConnection = new window.RTCPeerConnection(
      this.buildPeerConnectionConfig()
    );

    this.webrtcPeerConnection.ontrack = (event) => {
      const [remoteStream] = event.streams;
      if (this.videoRef.current && remoteStream) {
        this.videoRef.current.srcObject = remoteStream;
        const playPromise = this.videoRef.current.play();
        if (playPromise && playPromise.catch) {
          playPromise.catch(() => {});
        }
      }

      this.setState({
        liveviewLoaded: true,
      });
    };

    this.webrtcPeerConnection.onicecandidate = (event) => {
      if (!event.candidate) {
        return;
      }

      this.sendWebRTCMessage('webrtc-candidate', {
        session_id: this.webrtcSessionId,
        candidate: JSON.stringify(event.candidate.toJSON()),
      });
    };

    this.webrtcPeerConnection.onconnectionstatechange = () => {
      const { connectionState } = this.webrtcPeerConnection;
      if (connectionState === 'connected') {
        this.setState({
          liveviewLoaded: true,
        });
      }

      if (
        connectionState === 'failed' ||
        connectionState === 'disconnected' ||
        connectionState === 'closed'
      ) {
        this.fallbackToSDLiveview(
          `WebRTC connection ${connectionState}, falling back to SD liveview.`
        );
      }
    };

    this.webrtcSocket = new WebSocket(config.WS_URL);
    this.webrtcSocket.onopen = () => {
      this.sendWebRTCMessage('hello', {});
    };
    this.webrtcSocket.onmessage = this.handleWebRTCSignalMessage;
    this.webrtcSocket.onerror = () => {
      this.fallbackToSDLiveview('Unable to open the WebRTC signaling channel.');
    };
    this.webrtcSocket.onclose = () => {
      const { liveviewLoaded } = this.state;
      if (!liveviewLoaded) {
        this.fallbackToSDLiveview('WebRTC signaling channel closed early.');
      }
    };

    this.webrtcTimeout = window.setTimeout(() => {
      const { liveviewLoaded } = this.state;
      if (!liveviewLoaded) {
        this.fallbackToSDLiveview(
          'WebRTC connection timed out, falling back to SD liveview.'
        );
      }
    }, 10000);

    this.setState({
      initialised: true,
      liveviewLoaded: false,
      liveviewMode: 'webrtc',
    });
  }

  fallbackToSDLiveview(errorMessage) {
    const { liveviewMode } = this.state;

    if (liveviewMode === 'sd' && this.requestStreamSubscription) {
      return;
    }

    this.stopWebRTCLiveview();
    if (errorMessage) {
      // eslint-disable-next-line no-console
      console.warn(errorMessage);
    }

    this.setState(
      {
        initialised: true,
        liveviewLoaded: false,
        liveviewMode: 'sd',
      },
      () => {
        this.initialiseSDLiveview();
      }
    );
  }

  openModal(file) {
    this.setState({
      open: true,
      currentRecording: file,
    });
  }

  render() {
    const { dashboard, t, images } = this.props;
    const { liveviewLoaded, liveviewMode, open, currentRecording } = this.state;
    const listenerCount = dashboard.webrtcReaders ? dashboard.webrtcReaders : 0;

    // We check if the camera was getting a valid frame
    // during the last 5 seconds, otherwise we assume the camera is offline.
    const isCameraOnline = dashboard.cameraOnline;

    // We check if a connection is made to Kerberos Hub, or if Offline mode
    // has been turned on.
    const isCloudOnline = dashboard.cloudOnline;
    let cloudConnection = t('dashboard.not_connected');
    if (dashboard.offlineMode === 'true') {
      cloudConnection = t('dashboard.offline_mode');
    } else {
      cloudConnection = isCloudOnline
        ? t('dashboard.connected')
        : t('dashboard.not_connected');
    }

    return (
      <div id="dashboard">
        <Breadcrumb
          title={t('dashboard.title')}
          level1={t('dashboard.heading')}
          level1Link=""
        >
          <Link to="/media">
            <Button
              label={t('breadcrumb.watch_recordings')}
              icon="media"
              type="default"
            />
          </Link>
          <Link to="/settings">
            <Button
              label={t('breadcrumb.configure')}
              icon="preferences"
              type={isCameraOnline ? 'neutral' : 'default'}
            />
          </Link>
        </Breadcrumb>

        <div className="stats grid-container --four-columns">
          <KPI
            number={dashboard.days ? dashboard.days.length : 0}
            divider="0"
            footer={t('dashboard.number_of_days')}
          />
          <KPI
            number={
              dashboard.numberOfRecordings ? dashboard.numberOfRecordings : 0
            }
            divider="0"
            footer={t('dashboard.total_recordings')}
          />
          <Link to="/settings">
            <Card
              title="IP Camera"
              subtitle={
                isCameraOnline
                  ? t('dashboard.connected')
                  : t('dashboard.not_connected')
              }
              footer="Camera"
              icon={isCameraOnline ? 'circle-check-big' : 'circle-cross-big'}
            />
          </Link>
          <Link to="/settings">
            <Card
              title="Kerberos Hub"
              subtitle={cloudConnection}
              footer="Cloud"
              icon={
                isCloudOnline && dashboard.offlineMode !== 'true'
                  ? 'circle-check-big'
                  : 'circle-cross-big'
              }
            />
          </Link>
        </div>
        <hr />
        <div className="stats grid-container --two-columns">
          <div>
            <h2>{t('dashboard.latest_events')}</h2>

            {(!dashboard.latestEvents ||
              dashboard.latestEvents.length === 0) && (
              <SetupBox
                dashed
                url="/settings"
                btnicon="preferences"
                btnlabel={t('dashboard.configure_connection')}
                header={t('dashboard.no_events')}
                text={t('dashboard.no_events_description')}
              />
            )}

            {dashboard.latestEvents && dashboard.latestEvents.length > 0 && (
              <Table>
                <TableHeader>
                  <TableRow
                    id="header"
                    headercells={[
                      t('dashboard.time'),
                      t('dashboard.description'),
                      t('dashboard.name'),
                    ]}
                  />
                </TableHeader>
                <TableBody>
                  {dashboard.latestEvents &&
                    dashboard.latestEvents.map((event) => (
                      <TableRow
                        key={event.timestamp}
                        id="cells1"
                        bodycells={[
                          <>
                            <div
                              className="time"
                              onClick={() =>
                                this.openModal(
                                  `${config.URL}/file/${event.key}`
                                )
                              }
                            >
                              <Ellipse status="success" />{' '}
                              <p data-tip="10m and 5s ago">{event.time}</p>
                            </div>
                          </>,
                          <>
                            <p
                              className="pointer event-description"
                              onClick={() =>
                                this.openModal(
                                  `${config.URL}/file/${event.key}`
                                )
                              }
                            >
                              {t('dashboard.motion_detected')}
                            </p>
                          </>,
                          <>
                            <span className="version">{event.camera_name}</span>
                            &nbsp;
                            <Icon label="cameras" />
                          </>,
                        ]}
                      />
                    ))}
                </TableBody>
                {open && (
                  <Modal>
                    <ModalHeader
                      title="View recording"
                      onClose={() => this.handleClose()}
                    />
                    <ModalBody>
                      <video controls autoPlay>
                        <source src={currentRecording} type="video/mp4" />
                      </video>
                    </ModalBody>
                    <ModalFooter
                      right={
                        <>
                          <a
                            href={currentRecording}
                            download="video"
                            target="_blank"
                            rel="noreferrer"
                          >
                            <Button
                              label="Download"
                              icon="download"
                              type="button"
                              buttonType="button"
                            />
                          </a>
                          <Button
                            label="Close"
                            icon="cross-circle"
                            type="button"
                            buttonType="button"
                            onClick={() => this.handleClose()}
                          />
                        </>
                      }
                    />
                  </Modal>
                )}
              </Table>
            )}
          </div>
          <div>
            <h2>
              {t('dashboard.live_view')} ({listenerCount})
            </h2>
            {(!liveviewLoaded || !isCameraOnline) && (
              <SetupBox
                btnicon="preferences"
                btnlabel={t('dashboard.configure_connection')}
                dashed
                url="/settings"
                header={t('dashboard.loading_live_view')}
                text={t('dashboard.loading_live_view_description')}
              />
            )}
            <div
              style={{
                visibility:
                  liveviewLoaded && isCameraOnline ? 'visible' : 'hidden',
              }}
            >
              {liveviewMode === 'webrtc' ? (
                <video ref={this.videoRef} autoPlay muted playsInline />
              ) : (
                <ImageCard
                  imageSrc={`data:image/png;base64, ${
                    images.length ? images[0] : ''
                  }`}
                  onerror=""
                />
              )}
            </div>
          </div>
        </div>
        <ReactTooltip />
      </div>
    );
  }
}

const mapStateToProps = (state /* , ownProps */) => ({
  dashboard: state.agent.dashboard,
  config: state.agent.config,
  connected: state.wss.connected,
  images: state.wss.images,
});

const mapDispatchToProps = (dispatch) => ({
  dispatchSend: (message) => dispatch(send(message)),
  dispatchGetConfig: (onSuccess, onError) =>
    dispatch(getConfig(onSuccess, onError)),
});

Dashboard.propTypes = {
  dashboard: PropTypes.object.isRequired,
  config: PropTypes.object.isRequired,
  connected: PropTypes.bool.isRequired,
  images: PropTypes.array.isRequired,
  t: PropTypes.func.isRequired,
  dispatchSend: PropTypes.func.isRequired,
  dispatchGetConfig: PropTypes.func.isRequired,
};

export default withTranslation()(
  withRouter(connect(mapStateToProps, mapDispatchToProps)(Dashboard))
);
