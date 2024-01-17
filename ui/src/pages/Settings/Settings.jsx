import React from 'react';
import PropTypes from 'prop-types';
import { withTranslation } from 'react-i18next';
import {
  Breadcrumb,
  ControlBar,
  Input,
  Button,
  Block,
  BlockHeader,
  BlockBody,
  BlockFooter,
  InfoBar,
  Dropdown,
  Tabs,
  Tab,
  Icon,
  Toggle,
} from '@kerberos-io/ui';
import { Link, withRouter } from 'react-router-dom';
import { connect } from 'react-redux';
import { interval } from 'rxjs';
import { send } from '@giantmachines/redux-websocket';
import ImageCanvas from '../../components/ImageCanvas/ImageCanvas';
import './Settings.scss';
import timezones from './timezones';
import {
  addRegion,
  updateRegion,
  removeRegion,
  saveConfig,
  verifyOnvif,
  verifyCamera,
  verifyHub,
  verifyPersistence,
  getConfig,
  updateConfig,
} from '../../actions/agent';

// eslint-disable-next-line react/prefer-stateless-function
class Settings extends React.Component {
  KERBEROS_VAULT = 'kstorage'; // @TODO needs to change

  KERBEROS_HUB = 's3'; // @TODO needs to change

  DROPBOX = 'dropbox';

  constructor() {
    super();
    this.state = {
      search: '',
      config: {},
      selectedTab: 'overview',
      mqttSuccess: false,
      mqttError: false,
      configSuccess: false,
      configError: false,
      verifyHubSuccess: false,
      verifyHubError: false,
      verifyHubErrorMessage: '',
      persistenceSuccess: false,
      persistenceError: false,
      verifyPersistenceSuccess: false,
      verifyPersistenceError: false,
      verifyPersistenceMessage: '',
      verifyCameraSuccess: false,
      verifyCameraError: false,
      verifyCameraMessage: '',
      verifyOnvifSuccess: false,
      verifyOnvifError: false,
      verifyOnvifErrorMessage: '',
      loading: false,
      loadingHub: false,
      loadingCamera: false,
    };
    this.storageTypes = [
      {
        label: 'Kerberos Hub',
        value: this.KERBEROS_HUB,
      },
      {
        label: 'Kerberos Vault',
        value: this.KERBEROS_VAULT,
      },
      {
        label: 'Dropbox',
        value: this.DROPBOX,
      },
    ];

    this.tags = {
      general: ['timezone', 'generic', 'general'],
      mqtt: ['mqtt', 'message broker', 'events', 'broker'],
      'stun-turn': ['webrtc', 'stun', 'turn', 'livestreaming'],
      'kerberos-hub': [
        'kerberos hub',
        'dashboard',
        'agents',
        'monitoring',
        'site',
      ],
      persistence: [
        'kerberos hub',
        'kerberos vault',
        'storage',
        'persistence',
        'gcp',
        'aws',
        'minio',
        'dropbox',
      ],
    };
    this.timezones = [];
    timezones.forEach((t) =>
      this.timezones.push({
        label: t.text,
        value: t.utc[0],
      })
    );

    this.changeTab = this.changeTab.bind(this);
    this.onUpdateField = this.onUpdateField.bind(this);
    this.onUpdateToggle = this.onUpdateToggle.bind(this);
    this.onUpdateNumberField = this.onUpdateNumberField.bind(this);
    this.onUpdateTimeline = this.onUpdateTimeline.bind(this);
    this.initialiseLiveview = this.initialiseLiveview.bind(this);
    this.verifyPersistenceSettings = this.verifyPersistenceSettings.bind(this);
    this.verifyHubSettings = this.verifyHubSettings.bind(this);
    this.verifyCameraSettings = this.verifyCameraSettings.bind(this);
    this.verifySubCameraSettings = this.verifySubCameraSettings.bind(this);
    this.calculateTimetable = this.calculateTimetable.bind(this);
    this.saveConfig = this.saveConfig.bind(this);
    this.onUpdateDropdown = this.onUpdateDropdown.bind(this);
    this.onAddRegion = this.onAddRegion.bind(this);
    this.onUpdateRegion = this.onUpdateRegion.bind(this);
    this.onDeleteRegion = this.onDeleteRegion.bind(this);
    this.verifyONVIF = this.verifyONVIF.bind(this);
  }

  componentDidMount() {
    const { dispatchGetConfig } = this.props;
    dispatchGetConfig((data) => {
      const { config } = data;
      this.setState((prevState) => ({
        ...prevState,
        config,
      }));
      this.calculateTimetable(config.timetable);
    });
    this.initialiseLiveview();
  }

  componentWillUnmount() {
    document.removeEventListener('keydown', this.escFunction, false);
    clearInterval(this.interval);

    const { dispatchSend } = this.props;
    const message = {
      message_type: 'stop-sd',
    };
    dispatchSend(message);
  }

  onAddRegion(device, id, polygon) {
    const { dispatchAddRegion } = this.props;
    dispatchAddRegion(id, polygon);
  }

  onUpdateRegion(device, id, polygon) {
    const { dispatchUpdateRegion } = this.props;
    dispatchUpdateRegion(id, polygon);
  }

  onDeleteRegion(device, id, polygon) {
    const { dispatchRemoveRegion } = this.props;
    dispatchRemoveRegion(id, polygon);
  }

  onUpdateField(type, name, event, object) {
    const { value } = event.target;
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: value,
    });
  }

  onUpdateToggle(type, name, event, object) {
    const { checked } = event.target;
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: checked ? 'true' : 'false',
    });
  }

  onUpdateDropdown(type, name, value, object) {
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: value,
    });
  }

  onUpdateNumberField(type, name, event, object) {
    const { value } = event.target;
    let number = parseInt(value, 10);
    number = Number.isNaN(number) ? 0 : number;
    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, {
      ...object,
      [name]: number,
    });
  }

  onUpdateTimeline(type, index, key, event, object) {
    const { value } = event.target;
    let seconds = -1;
    if (value) {
      const a = value.split(':'); // split it at the colons
      if (a.length === 2) {
        if (a[0] <= 24 && a[1] < 60) {
          seconds = +a[0] * 60 * 60 + +a[1] * 60;
          this.timetable[index][`${key}Full`] = value;
        }
      }
    }

    const { dispatchUpdateConfig } = this.props;
    dispatchUpdateConfig(type, [
      ...object.slice(0, index),
      {
        ...object[index],
        [key]: seconds,
      },
      ...object.slice(index + 1),
    ]);
  }

  initialiseLiveview() {
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

  calculateTimetable(timetable) {
    this.timetable = timetable;
    if (this.timetable) {
      for (let i = 0; i < timetable.length; i += 1) {
        const time = timetable[i];
        const { start1, start2, end1, end2 } = time;
        this.timetable[i].start1Full = this.convertSecondsToHourMinute(start1);
        this.timetable[i].start2Full = this.convertSecondsToHourMinute(start2);
        this.timetable[i].end1Full = this.convertSecondsToHourMinute(end1);
        this.timetable[i].end2Full = this.convertSecondsToHourMinute(end2);
      }
    }
  }

  convertSecondsToHourMinute(seconds) {
    let sec = seconds;
    let hours = Math.floor(sec / 3600);
    hours = hours.toString();
    if (hours < 10) {
      hours = `0${hours}`;
    }
    if (hours.length > 2) {
      hours = hours.slice(0, 2);
    }
    sec %= 3600;
    let minutes = Math.floor(sec / 60);
    minutes = minutes.toString();
    if (minutes < 10) {
      minutes = `0${minutes}`;
    }
    if (minutes.length > 2) {
      minutes = minutes.slice(0, 2);
    }
    return `${hours}:${minutes}`;
  }

  changeTab(tab) {
    this.setState({
      selectedTab: tab,
    });
  }

  saveConfig() {
    const { config, dispatchSaveConfig } = this.props;

    this.setState({
      verifyPersistenceSuccess: false,
      verifyPersistenceError: false,
      verifyHubSuccess: false,
      verifyHubError: false,
      configSuccess: false,
      configError: false,
      verifyOnvifSuccess: false,
      verifyOnvifError: false,
    });

    if (config) {
      dispatchSaveConfig(
        config.config,
        () => {
          this.setState({
            configSuccess: true,
            configError: false,
          });
        },
        () => {
          this.setState({
            configSuccess: false,
            configError: true,
          });
        }
      );
    }
  }

  verifyONVIF() {
    const { config, dispatchVerifyOnvif } = this.props;

    // Get camera configuration (subset of config).
    const cameraConfig = {
      onvif_xaddr: config.config.capture.ipcamera.onvif_xaddr,
      onvif_username: config.config.capture.ipcamera.onvif_username,
      onvif_password: config.config.capture.ipcamera.onvif_password,
    };

    this.setState({
      verifyOnvifSuccess: false,
      verifyOnvifError: false,
      verifyOnvifErrorMessage: '',
      verifyCameraSuccess: false,
      verifyCameraError: false,
      verifyCameraErrorMessage: '',
      configSuccess: false,
      configError: false,
      loadingCamera: false,
      loadingOnvif: true,
    });

    if (config) {
      dispatchVerifyOnvif(
        cameraConfig,
        () => {
          this.setState({
            verifyOnvifSuccess: true,
            verifyOnvifError: false,
            verifyOnvifErrorMessage: '',
            loadingOnvif: false,
          });
        },
        (error) => {
          this.setState({
            verifyOnvifSuccess: false,
            verifyOnvifError: true,
            verifyOnvifErrorMessage: error,
            loadingOnvif: false,
          });
        }
      );
    }
  }

  verifyHubSettings() {
    const { config, dispatchVerifyHub } = this.props;
    if (config) {
      // overriding global for testing.
      // hub_key: "xxx"
      // hub_private_key: "xxxx"
      // hub_site: "testsite"
      // hub_uri: "https://api.cloud.kerberos.io"

      this.setState({
        configSuccess: false,
        configError: false,
        verifyPersistenceSuccess: false,
        verifyPersistenceError: false,
        verifyHubSuccess: false,
        verifyHubError: false,
        verifyHubErrorMessage: '',
        hubSuccess: false,
        hubError: false,
        verifyCameraSuccess: false,
        verifyCameraError: false,
        verifyCameraErrorMessage: '',
        verifyOnvifSuccess: false,
        verifyOnvifError: false,
        loadingHub: true,
      });

      // .... test fields

      dispatchVerifyHub(
        config.config,
        () => {
          this.setState({
            verifyHubSuccess: true,
            verifyHubError: false,
            verifyHubErrorMessage: '',
            hubSuccess: false,
            hubError: false,
            loadingHub: false,
          });
        },
        (error) => {
          this.setState({
            verifyHubSuccess: false,
            verifyHubError: true,
            verifyHubErrorMessage: error,
            hubSuccess: false,
            hubError: false,
            loadingHub: false,
          });
        }
      );
    }
  }

  verifyPersistenceSettings() {
    const { config, dispatchVerifyPersistence } = this.props;
    if (config) {
      this.setState({
        configSuccess: false,
        configError: false,
        verifyHubSuccess: false,
        verifyHubError: false,
        verifyPersistenceSuccess: false,
        verifyPersistenceError: false,
        persistenceSuccess: false,
        persistenceError: false,
        verifyCameraSuccess: false,
        verifyCameraError: false,
        verifyOnvifSuccess: false,
        verifyOnvifError: false,
        verifyCameraErrorMessage: '',
        loading: true,
      });

      dispatchVerifyPersistence(
        config.config,
        () => {
          this.setState({
            verifyPersistenceSuccess: true,
            verifyPersistenceError: false,
            verifyPersistenceMessage: '',
            persistenceSuccess: false,
            persistenceError: false,
            loading: false,
          });
        },
        (error) => {
          this.setState({
            verifyPersistenceSuccess: false,
            verifyPersistenceError: true,
            verifyPersistenceMessage: error,
            persistenceSuccess: false,
            persistenceError: false,
            loading: false,
          });
        }
      );
    }
  }

  verifySubCameraSettings(event) {
    this.verifyCameraSettings(event, 'secondary');
  }

  verifyCameraSettings(event, streamType = 'primary') {
    const { config, dispatchVerifyCamera } = this.props;
    if (config) {
      this.setState({
        configSuccess: false,
        configError: false,
        loadingCamera: true,
        verifyPersistenceSuccess: false,
        verifyPersistenceError: false,
        verifyHubSuccess: false,
        verifyHubError: false,
        verifyHubErrorMessage: '',
        verifyCameraSuccess: false,
        verifyCameraError: false,
        verifyCameraErrorMessage: '',
        verifyOnvifSuccess: false,
        verifyOnvifError: false,
        hubSuccess: false,
        hubError: false,
      });

      dispatchVerifyCamera(
        streamType,
        config.config,
        () => {
          this.setState({
            verifyCameraSuccess: true,
            verifyCameraError: false,
            verifyCameraErrorMessage: '',
            loadingCamera: false,
          });
        },
        (error) => {
          this.setState({
            verifyCameraSuccess: false,
            verifyCameraError: true,
            verifyCameraErrorMessage: error,
            loadingCamera: false,
          });
        }
      );
    }
  }

  render() {
    const {
      selectedTab,
      search,
      configSuccess,
      configError,
      verifyHubSuccess,
      verifyHubError,
      verifyHubErrorMessage,
      verifyPersistenceSuccess,
      verifyPersistenceError,
      verifyPersistenceMessage,
      verifyCameraSuccess,
      verifyCameraError,
      verifyCameraErrorMessage,
      loadingOnvif,
      verifyOnvifSuccess,
      verifyOnvifError,
      verifyOnvifErrorMessage,
      loadingCamera,
      loading,
      loadingHub,
    } = this.state;

    const { config: c, t, images } = this.props;
    const { config } = c;

    const snapshotBase64 = 'data:image/png;base64,';
    // Determine which section(s) to be shown, depending on the searching criteria.
    let showOverviewSection = false;
    let showRecordingSection = false;
    let showCameraSection = false;
    let showStreamingSection = false;
    let showConditionsHubSection = false;
    let showPersistenceSection = false;

    switch (selectedTab) {
      case 'all':
        showOverviewSection = true;
        showCameraSection = true;
        showRecordingSection = true;
        showStreamingSection = true;
        showConditionsHubSection = true;
        showPersistenceSection = true;
        break;
      case 'overview':
        showOverviewSection = true;
        break;
      case 'camera':
        showCameraSection = true;
        break;
      case 'recording':
        showRecordingSection = true;
        break;
      case 'streaming':
        showStreamingSection = true;
        break;
      case 'conditions':
        showConditionsHubSection = true;
        break;
      case 'persistence':
        showPersistenceSection = true;
        break;
      default:
    }

    if (search !== '' && search !== null) {
      if (this.tags) {
        const sections = Object.keys(this.tags);
        sections.forEach((section) => {
          // Find a match for the current section
          const sectionTags = this.tags[section];
          let match = false;
          sectionTags.forEach((tag) => {
            if (tag.toLowerCase().includes(search.toLowerCase())) {
              match = true;
            }
          });

          switch (section) {
            case 'overview':
              showOverviewSection = match;
              break;
            case 'camera':
              showCameraSection = match;
              break;
            case 'recording':
              showRecordingSection = match;
              break;
            case 'streaming':
              showStreamingSection = match;
              break;
            case 'conditions':
              showConditionsHubSection = match;
              break;
            case 'persistence':
              showPersistenceSection = match;
              break;
            default:
          }
        });
      }
    }

    return config ? (
      <div id="settings">
        <Breadcrumb
          title={t('settings.title')}
          level1={t('settings.heading')}
          level1Link=""
        >
          <Link to="/media">
            <Button
              label={t('breadcrumb.watch_recordings')}
              icon="media"
              type="default"
            />
          </Link>
        </Breadcrumb>
        <ControlBar type="row">
          <Tabs>
            <Tab
              label={t('settings.submenu.all')}
              value="all"
              active={selectedTab === 'all'}
              onClick={() => this.changeTab('all')}
              icon={<Icon label="list" />}
            />
            <Tab
              label={t('settings.submenu.overview')}
              value="overview"
              active={selectedTab === 'overview'}
              onClick={() => this.changeTab('overview')}
              icon={<Icon label="dashboard" />}
            />
            <Tab
              label={t('settings.submenu.camera')}
              value="camera"
              active={selectedTab === 'camera'}
              onClick={() => this.changeTab('camera')}
              icon={<Icon label="cameras" />}
            />
            <Tab
              label={t('settings.submenu.recording')}
              value="recording"
              active={selectedTab === 'recording'}
              onClick={() => this.changeTab('recording')}
              icon={<Icon label="refresh" />}
            />
            {config.offline !== 'true' && (
              <Tab
                label={t('settings.submenu.streaming')}
                value="streaming"
                active={selectedTab === 'streaming'}
                onClick={() => this.changeTab('streaming')}
                icon={<Icon label="livestream" />}
              />
            )}
            <Tab
              label={t('settings.submenu.conditions')}
              value="conditions"
              active={selectedTab === 'conditions'}
              onClick={() => this.changeTab('conditions')}
              icon={<Icon label="activity" />}
            />
            {config.offline !== 'true' && (
              <Tab
                label={t('settings.submenu.persistence')}
                value="persistence"
                active={selectedTab === 'persistence'}
                onClick={() => this.changeTab('persistence')}
                icon={<Icon label="cloud" />}
              />
            )}
          </Tabs>
        </ControlBar>

        {showPersistenceSection && config.offline !== 'true' && (
          <a
            href="https://app-demo.kerberos.io"
            target="_blank"
            rel="noreferrer"
          >
            <InfoBar
              type="info"
              message={t('settings.info.kerberos_hub_demo')}
            />
          </a>
        )}

        {configSuccess && (
          <InfoBar
            type="success"
            message={t('settings.info.configuration_updated_success')}
          />
        )}

        {configError && (
          <InfoBar
            type="alert"
            message={t('settings.info.configuration_updated_error')}
          />
        )}

        {loadingCamera && (
          <InfoBar type="loading" message={t('settings.info.verify_camera')} />
        )}
        {verifyCameraSuccess && (
          <InfoBar
            type="success"
            message={t('settings.info.verify_camera_success')}
          />
        )}
        {verifyCameraError && (
          <InfoBar
            type="alert"
            message={`${t(
              'settings.info.verify_camera_error'
            )}: ${verifyCameraErrorMessage}`}
          />
        )}

        {loadingOnvif && (
          <InfoBar type="loading" message={t('settings.info.verify_onvif')} />
        )}
        {verifyOnvifSuccess && (
          <InfoBar
            type="success"
            message={t('settings.info.verify_onvif_success')}
          />
        )}
        {verifyOnvifError && (
          <InfoBar type="alert" message={verifyOnvifErrorMessage} />
        )}

        {loadingHub && (
          <InfoBar type="loading" message={t('settings.info.verify_hub')} />
        )}
        {verifyHubSuccess && (
          <InfoBar
            type="success"
            message={t('settings.info.verify_hub_success')}
          />
        )}
        {verifyHubError && (
          <InfoBar
            type="alert"
            message={`${t(
              'settings.info.verify_hub_error'
            )} :${verifyHubErrorMessage}`}
          />
        )}

        {loading && (
          <InfoBar
            type="loading"
            message={t('settings.info.verify_persistence')}
          />
        )}
        {verifyPersistenceSuccess && (
          <InfoBar
            type="success"
            message={t('settings.info.verify_persistence_success')}
          />
        )}
        {verifyPersistenceError && (
          <InfoBar
            type="alert"
            message={`${t(
              'settings.info.verify_persistence_error'
            )} :${verifyPersistenceMessage}`}
          />
        )}
        <div className="stats grid-container --two-columns">
          <div>
            {/* General settings block */}
            {showOverviewSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.overview.general')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.overview.description_general')}</p>
                  <Input
                    noPadding
                    label={t('settings.overview.key')}
                    disabled
                    defaultValue={config.key}
                  />

                  <Input
                    noPadding
                    label={t('settings.overview.camera_name')}
                    defaultValue={config.name}
                    onChange={(value) =>
                      this.onUpdateField('', 'name', value, config)
                    }
                  />

                  <Dropdown
                    isRadio
                    icon="world"
                    label={t('settings.overview.timezone')}
                    placeholder={t('settings.overview.select_timezone')}
                    items={this.timezones}
                    selected={[config.timezone]}
                    shorten
                    shortenType="end"
                    shortenMaxLength={30}
                    onChange={(value) =>
                      this.onUpdateDropdown('', 'timezone', value[0], config)
                    }
                  />
                  <br />
                  <hr />
                  <p>
                    {t('settings.overview.description_advanced_configuration')}
                  </p>
                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.offline === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle('', 'offline', event, config)
                      }
                    />
                    <div>
                      <span>{t('settings.overview.offline_mode')}</span>
                      <p>{t('settings.overview.description_offline_mode')}</p>
                    </div>
                  </div>
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.save')}
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showCameraSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.camera.camera')}</h4>
                </BlockHeader>
                <BlockBody>
                  <div className="warning-message">
                    <InfoBar
                      message={t('settings.camera.only_h264')}
                      type="info"
                    />
                  </div>
                  <p>{t('settings.camera.description_camera')}</p>
                  <Input
                    noPadding
                    label={t('settings.camera.rtsp_url')}
                    value={config.capture.ipcamera.rtsp}
                    placeholder={t('settings.camera.rtsp_h264')}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'rtsp',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />
                  <Input
                    noPadding
                    label={t('settings.camera.sub_rtsp_url')}
                    value={config.capture.ipcamera.sub_rtsp}
                    placeholder={t('settings.camera.sub_rtsp_h264')}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'sub_rtsp',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  {config.capture.ipcamera &&
                    config.capture.ipcamera.sub_rtsp &&
                    config.capture.ipcamera.sub_rtsp !== '' && (
                      <Button
                        label={t('settings.camera.verify_sub_connection')}
                        disabled={loading}
                        onClick={this.verifySubCameraSettings}
                        type={loading ? 'neutral' : 'default'}
                        icon="verify"
                      />
                    )}
                  <Button
                    label={t('settings.camera.verify_connection')}
                    disabled={loading}
                    onClick={this.verifyCameraSettings}
                    type={loading ? 'neutral' : 'default'}
                    icon="verify"
                  />
                  <Button
                    label={t('buttons.save')}
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showRecordingSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.recording.recording')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.recording.description_recording')}</p>
                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.capture.continuous === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'continuous',
                          event,
                          config.capture
                        )
                      }
                    />
                    <div>
                      <span>
                        {t('settings.recording.continuous_recording')}
                      </span>
                      <p>
                        {t(
                          'settings.recording.description_continuous_recording'
                        )}
                      </p>
                    </div>
                  </div>

                  <Input
                    noPadding
                    label={t('settings.recording.max_duration')}
                    value={config.capture.maxlengthrecording}
                    placeholder={t(
                      'settings.recording.description_max_duration'
                    )}
                    onChange={(value) =>
                      this.onUpdateNumberField(
                        'capture',
                        'maxlengthrecording',
                        value,
                        config.capture
                      )
                    }
                  />

                  {config.capture.continuous !== 'true' && (
                    <>
                      <Input
                        noPadding
                        label={t('settings.recording.pre_recording')}
                        value={config.capture.prerecording}
                        placeholder={t('settings.recording.max_duration')}
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            'capture',
                            'prerecording',
                            value,
                            config.capture
                          )
                        }
                      />
                      <Input
                        noPadding
                        label={t('settings.recording.post_recording')}
                        value={config.capture.postrecording}
                        placeholder={t('settings.recording.post_recording')}
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            'capture',
                            'postrecording',
                            value,
                            config.capture
                          )
                        }
                      />
                      <Input
                        noPadding
                        label={t('settings.recording.threshold')}
                        value={config.capture.pixelChangeThreshold}
                        placeholder={t('settings.recording.threshold')}
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            'capture',
                            'pixelChangeThreshold',
                            value,
                            config.capture
                          )
                        }
                      />
                    </>
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.save')}
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* STUN/TURN block */}
            {showStreamingSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.streaming.stun_turn')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.streaming.description_stun_turn')}</p>
                  <Input
                    noPadding
                    label={t('settings.streaming.stun_server')}
                    value={config.stunuri}
                    onChange={(value) =>
                      this.onUpdateField('', 'stunuri', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label={t('settings.streaming.turn_server')}
                    value={config.turnuri}
                    onChange={(value) =>
                      this.onUpdateField('', 'turnuri', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label={t('settings.streaming.turn_username')}
                    value={config.turn_username}
                    onChange={(value) =>
                      this.onUpdateField('', 'turn_username', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label={t('settings.streaming.turn_password')}
                    value={config.turn_password}
                    onChange={(value) =>
                      this.onUpdateField('', 'turn_password', value, config)
                    }
                  />
                  <br />
                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.turn_force === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle('', 'turn_force', event, config)
                      }
                    />
                    <div>
                      <span>{t('settings.streaming.force_turn')}</span>
                      <p>{t('settings.streaming.force_turn_description')}</p>
                    </div>
                  </div>
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.save')}
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {showPersistenceSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.persistence.kerberoshub')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    {t('settings.persistence.description_kerberoshub')}{' '}
                    <a
                      href="https://doc.kerberos.io/hub/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub
                    </a>{' '}
                    {t('settings.persistence.description2_kerberoshub')}
                  </p>
                  <Input
                    noPadding
                    label={t('settings.persistence.kerberoshub_apiurl')}
                    placeholder={t(
                      'settings.persistence.kerberoshub_description_apiurl'
                    )}
                    value={config.hub_uri}
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_uri', value, config)
                    }
                  />
                  <Input
                    type="password"
                    iconright="activity"
                    label={t('settings.persistence.kerberoshub_publickey')}
                    placeholder={t(
                      'settings.persistence.kerberoshub_description_publickey'
                    )}
                    value={config.hub_key}
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_key', value, config)
                    }
                  />
                  <Input
                    type="password"
                    iconright="activity"
                    label={t('settings.persistence.kerberoshub_privatekey')}
                    placeholder={t(
                      'settings.persistence.kerberoshub_description_privatekey'
                    )}
                    value={config.hub_private_key}
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_private_key', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label={t('settings.persistence.kerberoshub_site')}
                    value={config.hub_site}
                    placeholder={t(
                      'settings.persistence.kerberoshub_description_site'
                    )}
                    onChange={(value) =>
                      this.onUpdateField('', 'hub_site', value, config)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('settings.persistence.verify_connection')}
                    disabled={loadingHub}
                    type={loadingHub ? 'neutral' : 'default'}
                    onClick={this.verifyHubSettings}
                    icon="verify"
                  />
                  <Button
                    label={t('buttons.save')}
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* Conditions block */}
            {showConditionsHubSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.conditions.regionofinterest')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.conditions.description_regionofinterest')}</p>
                  {config.region && images && images.length > 0 && (
                    <ImageCanvas
                      image={snapshotBase64 + images[0]}
                      polygons={config.region.polygon}
                      rendered={false}
                      onAddRegion={this.onAddRegion}
                      onUpdateRegion={this.onUpdateRegion}
                      onDeleteRegion={this.onDeleteRegion}
                    />
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.save')}
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}
          </div>

          <div>
            {/* General settings block */}
            {showCameraSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.camera.onvif')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.camera.description_onvif')}</p>

                  <Input
                    noPadding
                    label={t('settings.camera.onvif_xaddr')}
                    value={config.capture.ipcamera.onvif_xaddr}
                    placeholder="x.x.x.x:yyyy"
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_xaddr',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />

                  <Input
                    noPadding
                    label={t('settings.camera.onvif_username')}
                    value={config.capture.ipcamera.onvif_username}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_username',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />

                  <Input
                    noPadding
                    label={t('settings.camera.onvif_password')}
                    value={config.capture.ipcamera.onvif_password}
                    onChange={(value) =>
                      this.onUpdateField(
                        'capture.ipcamera',
                        'onvif_password',
                        value,
                        config.capture.ipcamera
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.verify_connection')}
                    type="default"
                    icon="verify"
                    onClick={this.verifyONVIF}
                  />
                  <Button
                    label={t('buttons.save')}
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showOverviewSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.overview.encryption')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.overview.description_encryption')}</p>
                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.encryption.enabled === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'encryption',
                          'enabled',
                          event,
                          config.encryption
                        )
                      }
                    />
                    <div>
                      <span>{t('settings.overview.encryption_enabled')}</span>
                      <p>
                        {t('settings.overview.description_encryption_enabled')}
                      </p>
                    </div>
                  </div>

                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.encryption.recordings === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'encryption',
                          'recordings',
                          event,
                          config.encryption
                        )
                      }
                    />
                    <div>
                      <span>
                        {t('settings.overview.encryption_recordings_enabled')}
                      </span>
                      <p>
                        {t(
                          'settings.overview.description_encryption_recordings_enabled'
                        )}
                      </p>
                    </div>
                  </div>

                  <Input
                    type="password"
                    iconright="activity"
                    label={t('settings.overview.encryption_fingerprint')}
                    value={config.encryption.fingerprint}
                    onChange={(value) =>
                      this.onUpdateField(
                        'encryption',
                        'fingerprint',
                        value,
                        config.encryption
                      )
                    }
                  />
                  <Input
                    type="password"
                    iconright="activity"
                    label={t('settings.overview.encryption_privatekey')}
                    value={config.encryption.private_key}
                    onChange={(value) =>
                      this.onUpdateField(
                        'encryption',
                        'private_key',
                        value,
                        config.encryption
                      )
                    }
                  />
                  <Input
                    type="password"
                    iconright="activity"
                    label={t('settings.overview.encryption_symmetrickey')}
                    value={config.encryption.symmetric_key}
                    onChange={(value) =>
                      this.onUpdateField(
                        'encryption',
                        'symmetric_key',
                        value,
                        config.encryption
                      )
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.save')}
                    type="default"
                    icon="pencil"
                    onClick={this.saveConfig}
                  />
                </BlockFooter>
              </Block>
            )}

            {showStreamingSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.streaming.mqtt')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    {t('settings.streaming.description_mqtt')}{' '}
                    <a
                      href="https://doc.kerberos.io/hub/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub
                    </a>{' '}
                    {t('settings.streaming.description2_mqtt')}
                  </p>
                  <Input
                    noPadding
                    label={t('settings.streaming.mqtt_brokeruri')}
                    value={config.mqtturi}
                    onChange={(value) =>
                      this.onUpdateField('', 'mqtturi', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label={t('settings.streaming.mqtt_username')}
                    value={config.mqtt_username}
                    onChange={(value) =>
                      this.onUpdateField('', 'mqtt_username', value, config)
                    }
                  />
                  <Input
                    noPadding
                    label={t('settings.streaming.mqtt_password')}
                    value={config.mqtt_password}
                    onChange={(value) =>
                      this.onUpdateField('', 'mqtt_password', value, config)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.save')}
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* STUN/TURN block */}
            {showStreamingSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.streaming.stun_turn_forward')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.streaming.stun_turn_description_forward')}</p>

                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.capture.forwardwebrtc === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'forwardwebrtc',
                          event,
                          config.capture
                        )
                      }
                    />
                    <div>
                      <span>{t('settings.streaming.stun_turn_webrtc')}</span>
                      <p>
                        {t('settings.streaming.stun_turn_description_webrtc')}
                      </p>
                    </div>
                  </div>

                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.capture.transcodingwebrtc === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          'capture',
                          'transcodingwebrtc',
                          event,
                          config.capture
                        )
                      }
                    />
                    <div>
                      <span>{t('settings.streaming.stun_turn_transcode')}</span>
                      <p>
                        {t(
                          'settings.streaming.stun_turn_description_transcode'
                        )}
                      </p>
                    </div>
                  </div>

                  {config.capture.transcodingwebrtc === 'true' && (
                    <Input
                      noPadding
                      label={t('settings.streaming.stun_turn_downscale')}
                      value={config.capture.transcodingresolution}
                      placeholder="The % of the original resolution."
                      onChange={(value) =>
                        this.onUpdateNumberField(
                          'capture',
                          'transcodingresolution',
                          value,
                          config.capture
                        )
                      }
                    />
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('buttons.save')}
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* General settings block */}
            {showRecordingSection && (
              <>
                <Block>
                  <BlockHeader>
                    <h4>{t('settings.recording.autoclean')}</h4>
                  </BlockHeader>
                  <BlockBody>
                    <p>{t('settings.recording.description_autoclean')}</p>

                    <div className="toggle-wrapper">
                      <Toggle
                        on={config.auto_clean === 'true'}
                        disabled={false}
                        onClick={(event) =>
                          this.onUpdateToggle('', 'auto_clean', event, config)
                        }
                      />
                      <div>
                        <span>{t('settings.recording.autoclean_enable')}</span>
                        <p>
                          {t('settings.recording.autoclean_description_enable')}
                        </p>
                      </div>
                    </div>

                    {config.auto_clean === 'true' && (
                      <Input
                        noPadding
                        label={t(
                          'settings.recording.autoclean_max_directory_size'
                        )}
                        value={config.max_directory_size}
                        placeholder={t(
                          'settings.recording.autoclean_description_enable'
                        )}
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            '',
                            'max_directory_size',
                            value,
                            config
                          )
                        }
                      />
                    )}
                  </BlockBody>
                  <BlockFooter>
                    <Button
                      label={t('buttons.save')}
                      type="default"
                      icon="pencil"
                      onClick={this.saveConfig}
                    />
                  </BlockFooter>
                </Block>
                <Block>
                  <BlockHeader>
                    <h4>{t('settings.recording.fragmentedrecordings')}</h4>
                  </BlockHeader>
                  <BlockBody>
                    <p>
                      {t('settings.recording.description_fragmentedrecordings')}
                    </p>

                    <div className="toggle-wrapper">
                      <Toggle
                        on={config.capture.fragmented === 'true'}
                        disabled={false}
                        onClick={(event) =>
                          this.onUpdateToggle(
                            'capture',
                            'fragmented',
                            event,
                            config.capture
                          )
                        }
                      />
                      <div>
                        <span>
                          {t('settings.recording.fragmentedrecordings_enable')}
                        </span>
                        <p>
                          {t(
                            'settings.recording.fragmentedrecordings_description_enable'
                          )}
                        </p>
                      </div>
                    </div>

                    {config.capture.fragmented === 'true' && (
                      <Input
                        noPadding
                        label={t(
                          'settings.recording.fragmentedrecordings_duration'
                        )}
                        value={config.capture.fragmentedduration}
                        placeholder={t(
                          'settings.recording.fragmentedrecordings_description_duration'
                        )}
                        onChange={(value) =>
                          this.onUpdateNumberField(
                            'capture',
                            'fragmentedduration',
                            value,
                            config.capture
                          )
                        }
                      />
                    )}
                  </BlockBody>
                  <BlockFooter>
                    <Button
                      label="Save"
                      type="default"
                      icon="pencil"
                      onClick={this.saveConfig}
                    />
                  </BlockFooter>
                </Block>
              </>
            )}

            {/* Conditions block */}
            {showConditionsHubSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.conditions.timeofinterest')}</h4>
                </BlockHeader>
                <BlockBody>
                  <div className="grid-2">
                    {this.timetable && this.timetable.length > 0 && (
                      <div>
                        <p>
                          {t('settings.conditions.description_timeofinterest')}
                        </p>

                        <div className="toggle-wrapper">
                          <Toggle
                            on={config.time === 'true'}
                            disabled={false}
                            onClick={(event) =>
                              this.onUpdateToggle('', 'time', event, config)
                            }
                          />
                          <div>
                            <span>
                              {t('settings.conditions.timeofinterest_enabled')}
                            </span>
                            <p>
                              {t(
                                'settings.conditions.timeofinterest_description_enabled'
                              )}
                            </p>
                          </div>
                        </div>

                        {config.time === 'true' && (
                          <div>
                            <span className="time-of-interest">
                              {t('settings.conditions.sunday')}
                            </span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[0].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[0].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[0].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[0].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    0,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">
                              {t('settings.conditions.monday')}
                            </span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[1].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[1].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[1].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[1].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    1,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">
                              {t('settings.conditions.tuesday')}
                            </span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[2].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[2].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[2].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[2].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    2,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">
                              {t('settings.conditions.wednesday')}
                            </span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[3].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[3].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[3].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[3].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    3,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">
                              {t('settings.conditions.thursday')}
                            </span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[4].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[4].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[4].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[4].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    4,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">
                              {t('settings.conditions.friday')}
                            </span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[5].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[5].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[5].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[5].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    5,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>

                            <span className="time-of-interest">
                              {t('settings.conditions.saturday')}
                            </span>
                            <div className="grid-4">
                              <Input
                                noPadding
                                placeholder="00:00"
                                value={this.timetable[6].start1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'start1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:00"
                                value={this.timetable[6].end1Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'end1',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="12:01"
                                value={this.timetable[6].start2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'start2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                              <Input
                                noPadding
                                placeholder="23:59"
                                value={this.timetable[6].end2Full}
                                onChange={(event) => {
                                  this.onUpdateTimeline(
                                    'timetable',
                                    6,
                                    'end2',
                                    event,
                                    config.timetable
                                  );
                                }}
                              />
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                </BlockBody>

                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* Conditions block */}
            {showConditionsHubSection && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.conditions.externalcondition')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>
                    {t('settings.conditions.description_externalcondition')}
                  </p>
                  <Input
                    noPadding
                    label="Condition URI"
                    value={config.condition_uri}
                    placeholder={
                      config.condition_uri
                        ? config.condition_uri
                        : 'The API for conditional recording (GET request).'
                    }
                    onChange={(value) =>
                      this.onUpdateField('', 'condition_uri', value, config)
                    }
                  />
                </BlockBody>

                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveConfig}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* Persistence block */}
            {showPersistenceSection && config.offline !== 'true' && (
              <Block>
                <BlockHeader>
                  <h4>{t('settings.persistence.persistence')}</h4>
                </BlockHeader>
                <BlockBody>
                  <p>{t('settings.persistence.remove_after_upload')}</p>
                  <div className="toggle-wrapper">
                    <Toggle
                      on={config.remove_after_upload === 'true'}
                      disabled={false}
                      onClick={(event) =>
                        this.onUpdateToggle(
                          '',
                          'remove_after_upload',
                          event,
                          config
                        )
                      }
                    />
                    <div>
                      <span>
                        {t('settings.persistence.remove_after_upload_enabled')}
                      </span>
                      <p>
                        {t(
                          'settings.persistence.remove_after_upload_description'
                        )}
                      </p>
                    </div>
                  </div>

                  <p>
                    {t('settings.persistence.description_persistence')}{' '}
                    <a
                      href="https://doc.kerberos.io/hub/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      {t('settings.persistence.saasoffering')}
                    </a>
                    ,{' '}
                    <a
                      href="https://doc.kerberos.io/vault/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Vault
                    </a>
                    {t('settings.persistence.description2_persistence')}.
                  </p>
                  <Dropdown
                    isRadio
                    icon="cloud"
                    placeholder={t('settings.persistence.select_persistence')}
                    selected={[config.cloud]}
                    items={this.storageTypes}
                    onChange={(value) =>
                      this.onUpdateDropdown('', 'cloud', value[0], config)
                    }
                  />
                  {config.cloud === this.KERBEROS_HUB && (
                    <>
                      <Input
                        noPadding
                        label={t('settings.persistence.kerberoshub_region')}
                        placeholder={t(
                          'settings.persistence.kerberoshub_description_region'
                        )}
                        value={config.s3 ? config.s3.region : ''}
                        onChange={(value) =>
                          this.onUpdateField('s3', 'region', value, config.s3)
                        }
                      />
                    </>
                  )}
                  {config.cloud === this.KERBEROS_VAULT && (
                    <>
                      <Input
                        noPadding
                        label={t('settings.persistence.kerberosvault_apiurl')}
                        placeholder={t(
                          'settings.persistence.kerberosvault_description_apiurl'
                        )}
                        value={config.kstorage ? config.kstorage.uri : ''}
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'uri',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        noPadding
                        label={t('settings.persistence.kerberosvault_provider')}
                        placeholder={t(
                          'settings.persistence.kerberosvault_description_provider'
                        )}
                        value={config.kstorage ? config.kstorage.provider : ''}
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'provider',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        noPadding
                        label={t(
                          'settings.persistence.kerberosvault_directory'
                        )}
                        placeholder={t(
                          'settings.persistence.kerberosvault_description_directory'
                        )}
                        value={config.kstorage ? config.kstorage.directory : ''}
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'directory',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        type="password"
                        iconright="activity"
                        label={t(
                          'settings.persistence.kerberosvault_accesskey'
                        )}
                        placeholder={t(
                          'settings.persistence.kerberosvault_description_accesskey'
                        )}
                        value={
                          config.kstorage ? config.kstorage.access_key : ''
                        }
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'access_key',
                            value,
                            config.kstorage
                          )
                        }
                      />
                      <Input
                        type="password"
                        iconright="activity"
                        label={t(
                          'settings.persistence.kerberosvault_secretkey'
                        )}
                        placeholder={t(
                          'settings.persistence.kerberosvault_description_secretkey'
                        )}
                        value={
                          config.kstorage
                            ? config.kstorage.secret_access_key
                            : ''
                        }
                        onChange={(value) =>
                          this.onUpdateField(
                            'kstorage',
                            'secret_access_key',
                            value,
                            config.kstorage
                          )
                        }
                      />
                    </>
                  )}
                  {config.cloud === this.DROPBOX && (
                    <>
                      <Input
                        noPadding
                        label={t('settings.persistence.dropbox_directory')}
                        placeholder={t(
                          'settings.persistence.dropbox_description_directory'
                        )}
                        value={config.dropbox ? config.dropbox.directory : ''}
                        onChange={(value) =>
                          this.onUpdateField(
                            'dropbox',
                            'directory',
                            value,
                            config.dropbox
                          )
                        }
                      />
                      <Input
                        noPadding
                        label={t('settings.persistence.dropbox_accesstoken')}
                        placeholder={t(
                          'settings.persistence.dropbox_description_accesstoken'
                        )}
                        value={
                          config.dropbox ? config.dropbox.access_token : ''
                        }
                        onChange={(value) =>
                          this.onUpdateField(
                            'dropbox',
                            'access_token',
                            value,
                            config.dropbox
                          )
                        }
                      />
                    </>
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label={t('settings.persistence.verify_connection')}
                    disabled={loading}
                    onClick={this.verifyPersistenceSettings}
                    type={loading ? 'neutral' : 'default'}
                    icon="verify"
                  />
                  <Button
                    label="Save"
                    type="submit"
                    onClick={this.saveConfig}
                    buttonType="submit"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}
          </div>
        </div>
      </div>
    ) : (
      <></>
    );
  }
}

const mapStateToProps = (state /* , ownProps */) => ({
  config: state.agent.config,
  connected: state.wss.connected,
  images: state.wss.images,
});

const mapDispatchToProps = (dispatch /* , ownProps */) => ({
  dispatchVerifyOnvif: (config, success, error) =>
    dispatch(verifyOnvif(config, success, error)),
  dispatchVerifyCamera: (streamType, config, success, error) =>
    dispatch(verifyCamera(streamType, config, success, error)),
  dispatchVerifyHub: (config, success, error) =>
    dispatch(verifyHub(config, success, error)),
  dispatchVerifyPersistence: (config, success, error) =>
    dispatch(verifyPersistence(config, success, error)),
  dispatchGetConfig: (callback) => dispatch(getConfig(callback)),
  dispatchUpdateConfig: (field, value) => dispatch(updateConfig(field, value)),
  dispatchSaveConfig: (config, success, error) =>
    dispatch(saveConfig(config, success, error)),
  dispatchAddRegion: (id, polygon) => dispatch(addRegion(id, polygon)),
  dispatchRemoveRegion: (id, polygon) => dispatch(removeRegion(id, polygon)),
  dispatchUpdateRegion: (id, polygon) => dispatch(updateRegion(id, polygon)),
  dispatchSend: (message) => dispatch(send(message)),
});

Settings.propTypes = {
  t: PropTypes.func.isRequired,
  connected: PropTypes.bool.isRequired,
  config: PropTypes.objectOf(PropTypes.object).isRequired,
  images: PropTypes.array.isRequired,
  dispatchVerifyHub: PropTypes.func.isRequired,
  dispatchVerifyPersistence: PropTypes.func.isRequired,
  dispatchGetConfig: PropTypes.func.isRequired,
  dispatchUpdateConfig: PropTypes.func.isRequired,
  dispatchSaveConfig: PropTypes.func.isRequired,
  dispatchAddRegion: PropTypes.func.isRequired,
  dispatchUpdateRegion: PropTypes.func.isRequired,
  dispatchRemoveRegion: PropTypes.func.isRequired,
  dispatchVerifyCamera: PropTypes.func.isRequired,
  dispatchVerifyOnvif: PropTypes.func.isRequired,
  dispatchSend: PropTypes.func.isRequired,
};

export default withTranslation()(
  withRouter(connect(mapStateToProps, mapDispatchToProps)(Settings))
);
